// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build !solaris,!darwin solaris,cgo darwin,cgo

package fswatcher

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/zillode/notify"
)

type fsEventType int

const (
	nonRemove fsEventType = 1
	remove                = 2
	mixed                 = 3
)

// Not meant to be changed, but must be changeable for tests
var (
	maxFiles       = 512
	maxFilesPerDir = 128
)

type fsEvent struct {
	path         string
	firstModTime time.Time
	lastModTime  time.Time
	eventType    fsEventType
}

type fsEventsBatch map[string]*fsEvent

type eventDir struct {
	path      string
	parentDir *eventDir
	events    fsEventsBatch
	dirs      map[string]*eventDir
}

func newEventDir(path string, parentDir *eventDir) *eventDir {
	return &eventDir{
		path:      path,
		parentDir: parentDir,
		events:    make(fsEventsBatch),
		dirs:      make(map[string]*eventDir),
	}
}

type fsWatcher struct {
	folderPath      string
	notifyModelChan chan []string
	// All detected and to be scanned events are stored in a tree like
	// structure mimicking folders to keep count of events per directory.
	rootEventDir *eventDir
	fsEventChan  chan notify.EventInfo
	// time interval to search for events to be passed to syncthing-core
	notifyDelay time.Duration
	// time after which an active event is passed to syncthing-core
	notifyTimeout         time.Duration
	notifyTimer           *time.Timer
	notifyTimerNeedsReset bool
	resetNotifyTimerChan  chan time.Duration
	inProgress            map[string]struct{}
	description           string
	ignores               *ignore.Matcher
	ignoresUpdate         chan *ignore.Matcher
	stop                  chan struct{}
	ignorePerms           bool
}

type Service interface {
	Serve()
	Stop()
	C() <-chan []string
	UpdateIgnores(ignores *ignore.Matcher)
}

func NewFsWatcher(cfg config.FolderConfiguration, ignores *ignore.Matcher) Service {
	fsWatcher := &fsWatcher{
		folderPath:            filepath.Clean(cfg.Path()),
		notifyModelChan:       make(chan []string),
		rootEventDir:          newEventDir(".", nil),
		fsEventChan:           make(chan notify.EventInfo, maxFiles),
		notifyDelay:           time.Duration(cfg.FsNotificationsDelayS) * time.Second,
		notifyTimeout:         notifyTimeout(cfg.FsNotificationsDelayS),
		notifyTimerNeedsReset: false,
		inProgress:            make(map[string]struct{}),
		description:           cfg.Description(),
		ignores:               ignores,
		ignoresUpdate:         make(chan *ignore.Matcher),
		resetNotifyTimerChan:  make(chan time.Duration),
		stop:                  make(chan struct{}),
		ignorePerms:           cfg.IgnorePerms,
	}

	if err := fsWatcher.setupNotifications(); err != nil {
		l.Warnf(`Starting filesystem notifications for folder %s: %v`, fsWatcher.description, err)
		return nil
	}

	return fsWatcher
}

func (w *fsWatcher) setupNotifications() error {
	absShouldIgnore := func(absPath string) bool {
		if !isSubpath(absPath, w.folderPath) {
			return true
		}
		relPath, _ := filepath.Rel(w.folderPath, absPath)
		return w.ignores.ShouldIgnore(relPath)
	}
	if err := notify.WatchWithFilter(filepath.Join(w.folderPath, "..."), w.fsEventChan, absShouldIgnore, w.eventMask()); err != nil {
		notify.Stop(w.fsEventChan)
		close(w.fsEventChan)
		if isWatchesTooFew(err) {
			err = watchesLimitTooLowError(w.description)
		}
		return err
	}
	l.Infoln("Started filesystem notifications for folder", w.description)
	return nil
}

func (w *fsWatcher) Serve() {
	w.notifyTimer = time.NewTimer(w.notifyDelay)
	defer w.notifyTimer.Stop()

	inProgressItemSubscription := events.Default.Subscribe(events.ItemStarted | events.ItemFinished)

	for {
		// Detect channel overflow
		if len(w.fsEventChan) == maxFiles {
		outer:
			for {
				select {
				case <-w.fsEventChan:
				default:
					break outer
				}
			}
			// Issue full rescan as events were lost
			w.newFsEvent(w.folderPath, nonRemove)
			l.Debugln(w, "Backend channel overflow: Scan entire folder")
		}
		select {
		case event, _ := <-w.fsEventChan:
			w.newFsEvent(event.Path(), w.eventType(event.Event()))
		case event := <-inProgressItemSubscription.C():
			w.updateInProgressSet(event)
		case <-w.notifyTimer.C:
			w.actOnTimer()
		case interval := <-w.resetNotifyTimerChan:
			w.resetNotifyTimer(interval)
		case ignores := <-w.ignoresUpdate:
			w.ignores = ignores
		case <-w.stop:
			return
		}
	}
}

func (w *fsWatcher) Stop() {
	close(w.stop)
	notify.Stop(w.fsEventChan)
	l.Infoln("Stopped filesystem notifications for folder", w.description)
}

func (w *fsWatcher) C() <-chan []string {
	return w.notifyModelChan
}

func (w *fsWatcher) newFsEvent(eventPath string, eventType fsEventType) {
	if _, ok := w.rootEventDir.events["."]; ok {
		l.Debugf("%v Will scan entire folder anyway; dropping: %s", w, eventPath)
		return
	}
	if isSubpath(eventPath, w.folderPath) {
		path, _ := filepath.Rel(w.folderPath, eventPath)
		if w.pathInProgress(path) {
			l.Debugf("%v Skipping notification for path we modified: %s", w, path)
			return
		}
		if w.ignores.ShouldIgnore(path) {
			l.Debugf("%v Ignoring: %s", w, path)
			return
		}
		w.aggregateEvent(path, time.Now(), eventType)
	} else {
		l.Debugf("%v Path outside of folder root: %s", w, eventPath)
		panic(fmt.Sprintf("bug: Detected change outside of root directory for folder %v", w.description))
	}
}

func isSubpath(path string, folderPath string) bool {
	return strings.HasPrefix(path, folderPath)
}

func (w *fsWatcher) resetNotifyTimerIfNeeded() {
	if w.notifyTimerNeedsReset {
		w.resetNotifyTimer(w.notifyDelay)
	}
}

// resetNotifyTimer should only ever be called when notifyTimer has stopped
// and notifyTimer.C been read from. Otherwise, call resetNotifyTimerIfNeeded.
func (w *fsWatcher) resetNotifyTimer(duration time.Duration) {
	l.Debugf("%v Resetting notifyTimer to %s", w, duration.String())
	w.notifyTimerNeedsReset = false
	w.notifyTimer.Reset(duration)
}

func (w *fsWatcher) aggregateEvent(path string, eventTime time.Time, eventType fsEventType) {
	if path == "." || w.rootEventDir.eventCount() == maxFiles {
		l.Debugln(w, "Scan entire folder")
		firstModTime := eventTime
		if w.rootEventDir.childCount() != 0 {
			eventType |= w.rootEventDir.getEventType()
			firstModTime = w.rootEventDir.getFirstModTime()
		}
		w.rootEventDir = newEventDir(".", nil)
		w.rootEventDir.events["."] = &fsEvent{
			path:         ".",
			firstModTime: firstModTime,
			lastModTime:  eventTime,
			eventType:    eventType,
		}
		w.resetNotifyTimerIfNeeded()
		return
	}

	parentDir := w.rootEventDir

	// Check if any parent directory is already tracked or will exceed
	// events per directory limit bottom up
	pathSegments := strings.Split(filepath.ToSlash(path), "/")

	// As root dir cannot be further aggregated, allow up to maxFiles
	// children.
	localMaxFilesPerDir := maxFiles
	var currPath string
	for i, pathSegment := range pathSegments[:len(pathSegments)-1] {
		currPath = filepath.Join(currPath, pathSegment)

		if event, ok := parentDir.events[currPath]; ok {
			event.lastModTime = eventTime
			event.eventType |= eventType
			l.Debugf("%v Parent %s (type %s) already tracked: %s", w, currPath, event.eventType, path)
			return
		}

		if parentDir.childCount() == localMaxFilesPerDir {
			l.Debugf("%v Parent dir %s already has %d children, tracking it instead: %s", w, currPath, localMaxFilesPerDir, path)
			w.aggregateEvent(filepath.Dir(currPath),
				eventTime, eventType)
			return
		}

		// If there are no events below path, but we need to recurse
		// into that path, create eventDir at path.
		if _, ok := parentDir.dirs[currPath]; !ok {
			l.Debugf("%v Creating eventDir: %s", w, currPath)
			parentDir.dirs[currPath] = newEventDir(currPath, parentDir)
		}
		parentDir = parentDir.dirs[currPath]

		// Reset allowed children count to maxFilesPerDir for non-root
		if i == 0 {
			localMaxFilesPerDir = maxFilesPerDir
		}
	}

	if event, ok := parentDir.events[path]; ok {
		event.lastModTime = eventTime
		event.eventType |= eventType
		l.Debugf("%v Already tracked (type %v): %s", w, event.eventType, path)
		return
	}

	childDir, ok := parentDir.dirs[path]

	// If a dir existed at path, it would be removed from dirs, thus
	// childCount would not increase.
	if !ok && parentDir.childCount() == localMaxFilesPerDir {
		l.Debugf("%v Parent dir already has %d children, tracking it instead: %s", w, localMaxFilesPerDir, path)
		w.aggregateEvent(filepath.Dir(path), eventTime, eventType)
		return
	}

	firstModTime := eventTime
	if ok {
		firstModTime = childDir.getFirstModTime()
		eventType |= childDir.getEventType()
		delete(parentDir.dirs, path)
	}
	l.Debugf("%v Tracking (type %v): %s", w, eventType, path)
	parentDir.events[path] = &fsEvent{
		path:         path,
		firstModTime: firstModTime,
		lastModTime:  eventTime,
		eventType:    eventType,
	}
	w.resetNotifyTimerIfNeeded()
}

func (w *fsWatcher) actOnTimer() {
	eventCount := w.rootEventDir.eventCount()
	if eventCount == 0 {
		l.Debugln(w, "No tracked events, waiting for new event.")
		w.notifyTimerNeedsReset = true
		return
	}
	oldFsEvents := w.popOldEvents(w.rootEventDir, time.Now())
	if len(oldFsEvents) == 0 {
		l.Debugln(w, "No old fs events")
		w.resetNotifyTimer(w.notifyDelay)
		return
	}
	// Sending to channel might block for a long time, but we need to keep
	// reading from notify backend channel to avoid overflow
	go func() {
		timeBeforeSending := time.Now()
		l.Debugf("%v Notifying about %d fs events", w, len(oldFsEvents))
		separatedBatches := make(map[fsEventType][]string)
		for path, event := range oldFsEvents {
			separatedBatches[event.eventType] = append(separatedBatches[event.eventType], path)
		}
		for _, eventType := range [3]fsEventType{nonRemove, mixed, remove} {
			if len(separatedBatches[eventType]) != 0 {
				w.notifyModelChan <- separatedBatches[eventType]
			}
		}
		// If sending to channel blocked for a long time,
		// shorten next notifyDelay accordingly.
		duration := time.Since(timeBeforeSending)
		buffer := time.Duration(1) * time.Millisecond
		switch {
		case duration < w.notifyDelay/10:
			w.resetNotifyTimerChan <- w.notifyDelay
		case duration+buffer > w.notifyDelay:
			w.resetNotifyTimerChan <- buffer
		default:
			w.resetNotifyTimerChan <- w.notifyDelay - duration
		}
	}()
	return
}

// popOldEvents removes events that should be sent to
func (w *fsWatcher) popOldEvents(dir *eventDir, currTime time.Time) fsEventsBatch {
	oldEvents := make(fsEventsBatch)
	for _, childDir := range dir.dirs {
		for path, event := range w.popOldEvents(childDir, currTime) {
			oldEvents[path] = event
		}
	}
	for path, event := range dir.events {
		// 2 * results in mean event age of notifyDelay (assuming randomly
		// occurring events). Reoccuring and remove (for efficient renames)
		// events are delayed until notifyTimeout.
		if (event.eventType != remove && 2*currTime.Sub(event.lastModTime) > w.notifyDelay) || currTime.Sub(event.firstModTime) > w.notifyTimeout {
			oldEvents[path] = event
			delete(dir.events, path)
		}
	}
	if dir.parentDir != nil && dir.childCount() == 0 {
		dir.parentDir.removeEmptyDir(dir.path)
	}
	return oldEvents
}

func (w *fsWatcher) updateInProgressSet(event events.Event) {
	if event.Type == events.ItemStarted {
		path := event.Data.(map[string]string)["item"]
		w.inProgress[path] = struct{}{}
	} else if event.Type == events.ItemFinished {
		path := event.Data.(map[string]interface{})["item"].(string)
		delete(w.inProgress, path)
	}
}

func (w *fsWatcher) pathInProgress(path string) bool {
	_, exists := w.inProgress[path]
	return exists
}

func (w *fsWatcher) UpdateIgnores(ignores *ignore.Matcher) {
	l.Debugln(w, "Ignore patterns update")
	w.ignoresUpdate <- ignores
}

func (w *fsWatcher) String() string {
	return fmt.Sprintf("fswatcher/%s:", w.description)
}

func (w *fsWatcher) eventType(notifyType notify.Event) fsEventType {
	if notifyType&w.removeEventMask() != 0 {
		return remove
	}
	return nonRemove
}

func (dir *eventDir) eventCount() int {
	count := len(dir.events)
	for _, dir := range dir.dirs {
		count += dir.eventCount()
	}
	return count
}

func (dir *eventDir) childCount() int {
	return len(dir.events) + len(dir.dirs)
}

func (dir *eventDir) removeEmptyDir(path string) {
	delete(dir.dirs, path)
	if dir.parentDir != nil && dir.childCount() == 0 {
		dir.parentDir.removeEmptyDir(dir.path)
	}
}

func watchesLimitTooLowError(folder string) error {
	// Exchange link for own documentation when available
	return fmt.Errorf("failed to install inotify handler for folder %s. Please increase inotify limits, see https://github.com/syncthing/syncthing-inotify#troubleshooting-for-folders-with-many-files-on-linux for more information", folder)
}

func (dir eventDir) getFirstModTime() time.Time {
	if dir.childCount() == 0 {
		panic("bug: getFirstModTime must not be used on empty eventDir")
	}
	firstModTime := time.Now()
	for _, childDir := range dir.dirs {
		dirTime := childDir.getFirstModTime()
		if dirTime.Before(firstModTime) {
			firstModTime = dirTime
		}
	}
	for _, event := range dir.events {
		if event.firstModTime.Before(firstModTime) {
			firstModTime = event.firstModTime
		}
	}
	return firstModTime
}

func (dir eventDir) getEventType() fsEventType {
	if dir.childCount() == 0 {
		panic("bug: getEventType must not be used on empty eventDir")
	}
	var eventType fsEventType
	for _, childDir := range dir.dirs {
		eventType |= childDir.getEventType()
		if eventType == mixed {
			return mixed
		}
	}
	for _, event := range dir.events {
		eventType |= event.eventType
		if eventType == mixed {
			return mixed
		}
	}
	return eventType
}

func notifyTimeout(eventDelayS int) time.Duration {
	shortDelayS := 10
	shortDelayMultiplicator := 6
	longDelayS := 60
	longDelayTimeout := time.Duration(1) * time.Minute
	if eventDelayS < shortDelayS {
		return time.Duration(eventDelayS*shortDelayMultiplicator) * time.Second
	}
	if eventDelayS < longDelayS {
		return longDelayTimeout
	}
	return time.Duration(eventDelayS) * time.Second
}

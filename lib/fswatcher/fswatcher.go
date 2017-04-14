// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package fswatcher

import (
	"errors"
	"fmt"
	"github.com/zillode/notify"
	"path/filepath"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/ignore"
)

type fsEventType int

const (
	nonRemove fsEventType = 1
	remove                = 2
	mixed                 = 3
)

type FsEvent struct {
	path         string
	firstModTime time.Time
	lastModTime  time.Time
	eventType    fsEventType
}

type FsEventsBatch map[string]*FsEvent

type eventDir struct {
	path      string
	parentDir *eventDir
	events    FsEventsBatch
	dirs      map[string]*eventDir
}

func newEventDir(path string, parentDir *eventDir) *eventDir {
	return &eventDir{
		path:      path,
		parentDir: parentDir,
		events:    make(FsEventsBatch),
		dirs:      make(map[string]*eventDir),
	}
}

type fsWatcher struct {
	folderPath      string
	notifyModelChan chan FsEventsBatch
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
	inProgress            map[string]struct{}
	description           string
	ignores               *ignore.Matcher
	ignoresUpdate         chan *ignore.Matcher
	resetTimerChan        chan time.Duration
	stop                  chan struct{}
	ignorePerms           bool
}

type Service interface {
	Serve()
	Stop()
	FsWatchChan() <-chan FsEventsBatch
	UpdateIgnores(ignores *ignore.Matcher)
}

// Not meant to be changed, but must be changeable for tests
var (
	maxFiles       = 512
	maxFilesPerDir = 128
)

func NewFsWatcher(cfg config.FolderConfiguration, ignores *ignore.Matcher) (Service, error) {
	fsWatcher := &fsWatcher{
		folderPath:            filepath.Clean(cfg.Path()),
		notifyModelChan:       make(chan FsEventsBatch),
		rootEventDir:          newEventDir(".", nil),
		fsEventChan:           make(chan notify.EventInfo, maxFiles),
		notifyDelay:           time.Duration(cfg.NotifyDelayS) * time.Second,
		notifyTimeout:         notifyTimeout(cfg.NotifyDelayS),
		notifyTimerNeedsReset: false,
		inProgress:            make(map[string]struct{}),
		description:           cfg.Description(),
		ignores:               ignores,
		ignoresUpdate:         make(chan *ignore.Matcher),
		resetTimerChan:        make(chan time.Duration),
		stop:                  make(chan struct{}),
		ignorePerms:           cfg.IgnorePerms,
	}

	if err := fsWatcher.setupNotifications(); err != nil {
		l.Warnf(`Folder "%s": Starting FS notifications failed: %s`,
			fsWatcher.description, err)
		return nil, err
	}

	return fsWatcher, nil
}

func (watcher *fsWatcher) setupNotifications() error {
	absShouldIgnore := func(absPath string) bool {
		if !isSubpath(absPath, watcher.folderPath) {
			return true
		}
		relPath, _ := filepath.Rel(watcher.folderPath, absPath)
		return watcher.ignores.ShouldIgnore(relPath)
	}
	// if err := notify.WatchWithFilter(filepath.Join(watcher.folderPath, "..."),
	if err := notify.WatchWithFilter(filepath.Join(watcher.folderPath, "..."),
		watcher.fsEventChan, absShouldIgnore, watcher.eventMask()); err != nil {
		notify.Stop(watcher.fsEventChan)
		close(watcher.fsEventChan)
		if isWatchesTooFew(err) {
			err = WatchesLimitTooLowError(watcher.description)
		}
		return err
	}
	l.Infoln(watcher, "Initiated filesystem watcher")
	return nil
}

func (watcher *fsWatcher) Serve() {
	watcher.notifyTimer = time.NewTimer(watcher.notifyDelay)
	defer watcher.notifyTimer.Stop()

	inProgressItemSubscription := events.Default.Subscribe(
		events.ItemStarted | events.ItemFinished)

	for {
		// Detect channel overflow
		if len(watcher.fsEventChan) == maxFiles {
		outer:
			for {
				select {
				case <-watcher.fsEventChan:
				default:
					break outer
				}
			}
			// Issue full rescan as events were lost
			watcher.newFsEvent(watcher.folderPath, nonRemove)
			l.Debugln(watcher, "Backend channel overflow: Scan entire folder")
		}
		select {
		case event, _ := <-watcher.fsEventChan:
			watcher.newFsEvent(event.Path(), eventType(event.Event()))
		case event := <-inProgressItemSubscription.C():
			watcher.updateInProgressSet(event)
		case <-watcher.notifyTimer.C:
			watcher.actOnTimer()
		case interval := <-watcher.resetTimerChan:
			watcher.resetNotifyTimer(interval)
		case ignores := <-watcher.ignoresUpdate:
			watcher.ignores = ignores
		case <-watcher.stop:
			return
		}
	}
}

func (watcher *fsWatcher) Stop() {
	close(watcher.stop)
	notify.Stop(watcher.fsEventChan)
	l.Infoln(watcher, "Stopped filesystem watcher")
}

func (watcher *fsWatcher) FsWatchChan() <-chan FsEventsBatch {
	return watcher.notifyModelChan
}

func (watcher *fsWatcher) newFsEvent(eventPath string, eventType fsEventType) {
	if _, ok := watcher.rootEventDir.events["."]; ok {
		l.Debugf("%v Will scan entire folder anyway; dropping: %s",
			watcher, eventPath)
		return
	}
	if isSubpath(eventPath, watcher.folderPath) {
		path, _ := filepath.Rel(watcher.folderPath, eventPath)
		if watcher.pathInProgress(path) {
			l.Debugf("%v Skipping notification for path we modified: %s",
				watcher, path)
			return
		}
		if watcher.ignores.ShouldIgnore(path) {
			l.Debugf("%v Ignoring: %s", watcher, path)
			return
		}
		watcher.aggregateEvent(path, time.Now(), eventType)
	} else {
		l.Warnf("%v Bug: Detected change outside of folder (%v), dropping: %s",
			watcher, watcher.folderPath, eventPath)
	}
}

func isSubpath(path string, folderPath string) bool {
	return strings.HasPrefix(path, folderPath)
}

func (watcher *fsWatcher) resetNotifyTimerIfNeeded() {
	if watcher.notifyTimerNeedsReset {
		watcher.resetNotifyTimer(watcher.notifyDelay)
	}
}

func (watcher *fsWatcher) resetNotifyTimer(duration time.Duration) {
	l.Debugf("%v Resetting notifyTimer to %s", watcher, duration.String())
	watcher.notifyTimerNeedsReset = false
	watcher.notifyTimer.Reset(duration)
}

func (watcher *fsWatcher) aggregateEvent(path string, eventTime time.Time, eventType fsEventType) {
	if path == "." || watcher.rootEventDir.eventCount() == maxFiles {
		l.Debugln(watcher, "Scan entire folder")
		firstModTime := eventTime
		if watcher.rootEventDir.childCount() != 0 {
			eventType |= watcher.rootEventDir.getEventType()
			firstModTime = watcher.rootEventDir.getFirstModTime()
		}
		watcher.rootEventDir = newEventDir(".", nil)
		watcher.rootEventDir.events["."] = &FsEvent{".", firstModTime,
			eventTime, eventType}
		watcher.resetNotifyTimerIfNeeded()
		return
	}

	parentDir := watcher.rootEventDir

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
			l.Debugf("%v Parent %s (type %s) already tracked: %s",
				watcher, currPath, event.eventType, path)
			return
		}

		if parentDir.childCount() == localMaxFilesPerDir {
			l.Debugf("%v Parent dir %s already has %d children,"+
				"tracking it instead: %s", watcher, currPath,
				localMaxFilesPerDir, path)
			watcher.aggregateEvent(filepath.Dir(currPath),
				eventTime, eventType)
			return
		}

		// If there are no events below path, but we need to recurse
		// into that path, create eventDir at path.
		if _, ok := parentDir.dirs[currPath]; !ok {
			l.Debugf("%v Creating eventDir: %s", watcher, currPath)
			parentDir.dirs[currPath] = newEventDir(currPath,
				parentDir)
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
		l.Debugf("%v Already tracked (type %v): %s", watcher,
			event.eventType, path)
		return
	}

	childDir, ok := parentDir.dirs[path]

	// If a dir existed at path, it would be removed from dirs, thus
	// childCount would not increase.
	if !ok && parentDir.childCount() == localMaxFilesPerDir {
		l.Debugf("%v Parent dir already has %d children, tracking it "+
			"instead: %s", watcher, localMaxFilesPerDir, path)
		watcher.aggregateEvent(filepath.Dir(path), eventTime, eventType)
		return
	}

	firstModTime := eventTime
	if ok {
		firstModTime = childDir.getFirstModTime()
		eventType |= childDir.getEventType()
		delete(parentDir.dirs, path)
	}
	l.Debugf("%v Tracking (type %v): %s", watcher, eventType, path)
	parentDir.events[path] = &FsEvent{path, firstModTime, eventTime, eventType}
	watcher.resetNotifyTimerIfNeeded()
}

func (watcher *fsWatcher) actOnTimer() {
	eventCount := watcher.rootEventDir.eventCount()
	if eventCount == 0 {
		l.Verboseln(watcher, "No tracked events, waiting for new event.")
		watcher.notifyTimerNeedsReset = true
		return
	}
	oldFsEvents := watcher.popOldEvents(watcher.rootEventDir, time.Now())
	// Sending to channel might block for a long time, but we need to keep
	// reading from notify backend channel to avoid overflow
	if len(oldFsEvents) != 0 {
		go func() {
			timeBeforeSending := time.Now()
			l.Verbosef("%v Notifying about %d fs events", watcher,
				len(oldFsEvents))
			separatedBatches := make(map[fsEventType]FsEventsBatch)
			separatedBatches[nonRemove] = make(FsEventsBatch)
			separatedBatches[mixed] = make(FsEventsBatch)
			separatedBatches[remove] = make(FsEventsBatch)
			for path, event := range oldFsEvents {
				separatedBatches[event.eventType][path] = event
			}
			for eventType := nonRemove; eventType <= mixed; eventType++ {
				if len(separatedBatches[eventType]) != 0 {
					watcher.notifyModelChan <- separatedBatches[eventType]
				}
			}
			// If sending to channel blocked for a long time,
			// shorten next notifyDelay accordingly.
			duration := time.Since(timeBeforeSending)
			buffer := time.Duration(1) * time.Millisecond
			switch {
			case duration < watcher.notifyDelay/10:
				watcher.resetTimerChan <- watcher.notifyDelay
			case duration+buffer > watcher.notifyDelay:
				watcher.resetTimerChan <- buffer
			default:
				watcher.resetTimerChan <- watcher.notifyDelay - duration
			}
		}()
		return
	}
	l.Verboseln(watcher, "No old fs events")
	watcher.resetNotifyTimer(watcher.notifyDelay)
}

func (watcher *fsWatcher) popOldEvents(dir *eventDir, currTime time.Time) FsEventsBatch {
	oldEvents := make(FsEventsBatch)
	for _, childDir := range dir.dirs {
		for path, event := range watcher.popOldEvents(childDir, currTime) {
			oldEvents[path] = event
		}
	}
	for path, event := range dir.events {
		// 2 * results in mean event age of notifyDelay (assuming randomly
		// occurring events). Reoccuring and remove (for efficient renames)
		// events are delayed until notifyTimeout.
		if (event.eventType != remove && 2*currTime.Sub(event.lastModTime) > watcher.notifyDelay) ||
			currTime.Sub(event.firstModTime) > watcher.notifyTimeout {
			oldEvents[path] = event
			delete(dir.events, path)
		}
	}
	if dir.parentDir != nil && dir.childCount() == 0 {
		dir.parentDir.removeEmptyDir(dir.path)
	}
	return oldEvents
}

func (watcher *fsWatcher) updateInProgressSet(event events.Event) {
	if event.Type == events.ItemStarted {
		path := event.Data.(map[string]string)["item"]
		watcher.inProgress[path] = struct{}{}
	} else if event.Type == events.ItemFinished {
		path := event.Data.(map[string]interface{})["item"].(string)
		delete(watcher.inProgress, path)
	}
}

func (watcher *fsWatcher) pathInProgress(path string) bool {
	_, exists := watcher.inProgress[path]
	return exists
}

func (watcher *fsWatcher) UpdateIgnores(ignores *ignore.Matcher) {
	l.Debugln(watcher, "Ignore patterns update")
	watcher.ignoresUpdate <- ignores
}

func (watcher *fsWatcher) String() string {
	return fmt.Sprintf("fswatcher/%s:", watcher.description)
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

func (batch FsEventsBatch) GetPaths() []string {
	var paths []string
	for _, event := range batch {
		paths = append(paths, event.path)
	}
	return paths
}

func WatchesLimitTooLowError(folder string) error {
	return errors.New("Failed to install inotify handler for " +
		folder +
		". Please increase inotify limits," +
		" see http://bit.ly/1PxkdUC for more information.")
}

func (dir eventDir) getFirstModTime() time.Time {
	if dir.childCount() == 0 {
		panic("getFirstModTime must not be used on empty eventDir")
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
		panic("getEventType must not be used on empty eventDir")
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

func (eventType fsEventType) String() string {
	switch {
	case eventType == nonRemove:
		return "nonRemove"
	case eventType == remove:
		return "remove"
	case eventType == mixed:
		return "mixed"
	default:
		panic("fswatcher: Unknown event type")
	}
}

func notifyTimeout(eventDelayS int) time.Duration {
	if eventDelayS < 10 {
		return time.Duration(eventDelayS*6) * time.Second
	}
	if eventDelayS < 60 {
		return time.Duration(1) * time.Minute
	}
	return time.Duration(eventDelayS) * time.Second
}

func eventType(notifyType notify.Event) fsEventType {
	if notifyType&removeEventMask != 0 {
		return remove
	}
	return nonRemove
}

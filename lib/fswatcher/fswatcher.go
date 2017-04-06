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

	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/ignore"
)

type FsEvent struct {
	path         string
	firstModTime time.Time
	lastModTime  time.Time
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
	folderID              string
	ignores               *ignore.Matcher
	ignoresUpdate         chan *ignore.Matcher
	resetTimerChan        chan time.Duration
	stop                  chan struct{}
}

type Service interface {
	Serve()
	Stop()
	FsWatchChan() <-chan FsEventsBatch
	UpdateIgnores(ignores *ignore.Matcher)
}

const (
	maxFiles       = 512
	maxFilesPerDir = 128
)

func NewFsWatcher(folderPath string, folderID string, ignores *ignore.Matcher,
	notifyDelayS int) (Service, error) {
	fsWatcher := &fsWatcher{
		folderPath:            folderPath,
		notifyModelChan:       make(chan FsEventsBatch),
		rootEventDir:          newEventDir(".", nil),
		fsEventChan:           make(chan notify.EventInfo, maxFiles),
		notifyDelay:           time.Duration(notifyDelayS) * time.Second,
		notifyTimeout:         notifyTimeout(notifyDelayS),
		notifyTimerNeedsReset: false,
		inProgress:            make(map[string]struct{}),
		folderID:              folderID,
		ignores:               ignores,
		ignoresUpdate:         make(chan *ignore.Matcher),
		resetTimerChan:        make(chan time.Duration),
		stop:                  make(chan struct{}),
	}

	if err := fsWatcher.setupNotifications(); err != nil {
		l.Warnf(`Folder "%s": Starting FS notifications failed: %s`,
			fsWatcher.folderID, err)
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
	if err := notify.WatchWithFilter(filepath.Join(watcher.folderPath, "..."),
		watcher.fsEventChan, absShouldIgnore, notify.All); err != nil {
		notify.Stop(watcher.fsEventChan)
		close(watcher.fsEventChan)
		if isWatchesTooFew(err) {
			err = WatchesLimitTooLowError(watcher.folderID)
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
			watcher.newFsEvent(".")
		}
		select {
		case event, _ := <-watcher.fsEventChan:
			watcher.newFsEvent(event.Path())
		case event := <-inProgressItemSubscription.C():
			watcher.updateInProgressSet(event)
		case <-watcher.notifyTimer.C:
			watcher.actOnTimer()
		case interval := <-watcher.resetTimerChan:
			watcher.resetNotifyTimer(interval)
		case ignores := <-watcher.ignoresUpdate:
			watcher.ignores = ignores
		case <-watcher.stop:
			notify.Stop(watcher.fsEventChan)
			return
		}
	}
}

func (watcher *fsWatcher) Stop() {
	close(watcher.stop)
	l.Infoln(watcher, "Stopped filesystem watcher")
}

func (watcher *fsWatcher) FsWatchChan() <-chan FsEventsBatch {
	return watcher.notifyModelChan
}

func (watcher *fsWatcher) newFsEvent(eventPath string) {
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
		watcher.aggregateEvent(path, time.Now())
	} else {
		l.Warnf("%v Bug: Detected change outside of folder, dropping: %s",
			watcher, eventPath)
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

func (watcher *fsWatcher) aggregateEvent(path string, eventTime time.Time) {
	if path == "." || watcher.rootEventDir.eventCount() == maxFiles {
		l.Debugln(watcher, "Scan entire folder")
		firstModTime := eventTime
		if watcher.rootEventDir.childCount() != 0 {
			firstModTime = watcher.rootEventDir.getFirstModTime()
		}
		watcher.rootEventDir = newEventDir(".", nil)
		watcher.rootEventDir.events["."] = &FsEvent{".", firstModTime,
			eventTime}
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
			l.Debugf("%v Parent %s already tracked: %s", watcher,
				currPath, path)
			return
		}

		if parentDir.childCount() == localMaxFilesPerDir {
			l.Debugf("%v Parent dir %s already has %d children,"+
				"tracking it instead: %s", watcher, currPath,
				localMaxFilesPerDir, path)
			watcher.aggregateEvent(filepath.Dir(currPath),
				eventTime)
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
		l.Debugf("%v Already tracked: %s", watcher, path)
		return
	}

	childDir, ok := parentDir.dirs[path]

	// If a dir existed at path, it would be removed from dirs, thus
	// childCount would not increase.
	if !ok && parentDir.childCount() == localMaxFilesPerDir {
		l.Debugf("%v Parent dir already has %d children, tracking it "+
			"instead: %s", watcher, localMaxFilesPerDir, path)
		watcher.aggregateEvent(filepath.Dir(path), eventTime)
		return
	}

	firstModTime := eventTime
	if ok {
		firstModTime = childDir.getFirstModTime()
		delete(parentDir.dirs, path)
	}
	l.Debugf("%v Tracking: %s", watcher, path)
	parentDir.events[path] = &FsEvent{path, firstModTime, eventTime}
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
			watcher.notifyModelChan <- oldFsEvents
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
		// 2 * results in mean event age of notifyDelay
		// (assuming randomly occurring events)
		if 2*currTime.Sub(event.lastModTime) > watcher.notifyDelay ||
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
	return fmt.Sprintf("fswatcher/%s:", watcher.folderID)
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

func notifyTimeout(eventDelayS int) time.Duration {
	if eventDelayS < 12 {
		return time.Duration(eventDelayS*5) * time.Second
	}
	if eventDelayS < 60 {
		return time.Duration(1) * time.Minute
	}
	return time.Duration(eventDelayS) * time.Second
}

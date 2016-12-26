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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/ignore"
)

type FsEvent struct {
	path string
	time time.Time
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

type FsWatcher struct {
	folderPath      string
	notifyModelChan chan<- FsEventsBatch
	// All detected and to be scanned events are stored in a tree like
	// structure mimicking folders to keep count of events per directory.
	rootEventDir          *eventDir
	fsEventChan           chan notify.EventInfo
	WatchingFs            bool
	notifyDelay           time.Duration
	slowNotifyDelay       time.Duration
	notifyTimer           *time.Timer
	notifyTimerNeedsReset bool
	inProgress            map[string]struct{}
	folderID              string
	ignores               *ignore.Matcher
	ignoresUpdate         chan *ignore.Matcher
	stop                  chan struct{}
}

const (
	fastNotifyDelay = time.Duration(500) * time.Millisecond
	maxFiles        = 512
	maxFilesPerDir  = 128
)

func NewFsWatcher(folderPath string, folderID string, ignores *ignore.Matcher,
	slowNotifyDelayS int) *FsWatcher {
	if slowNotifyDelayS == 0 {
		slowNotifyDelayS = 60 * 60 * 24
	}
	return &FsWatcher{
		folderPath:            folderPath,
		notifyModelChan:       nil,
		rootEventDir:          newEventDir(".", nil),
		fsEventChan:           nil,
		WatchingFs:            false,
		notifyDelay:           fastNotifyDelay,
		slowNotifyDelay:       time.Duration(slowNotifyDelayS) * time.Second,
		notifyTimerNeedsReset: false,
		inProgress:            make(map[string]struct{}),
		folderID:              folderID,
		ignores:               ignores,
		ignoresUpdate:         make(chan *ignore.Matcher),
		stop:                  make(chan struct{}),
	}
}

func (watcher *FsWatcher) StartWatchingFilesystem() (<-chan FsEventsBatch, error) {
	fsEventChan, err := watcher.setupNotifications()
	notifyModelChan := make(chan FsEventsBatch)
	watcher.notifyModelChan = notifyModelChan
	if err == nil {
		watcher.WatchingFs = true
		watcher.fsEventChan = fsEventChan
		go watcher.watchFilesystem()
	}
	return notifyModelChan, err
}

func (watcher *FsWatcher) setupNotifications() (chan notify.EventInfo, error) {
	c := make(chan notify.EventInfo, maxFiles)
	absShouldIgnore := func(absPath string) bool {
		relPath, _ := filepath.Rel(watcher.folderPath, absPath)
		return watcher.ignores.ShouldIgnore(relPath)
	}
	if err := notify.WatchWithFilter(filepath.Join(watcher.folderPath, "..."),
		c, absShouldIgnore, notify.All); err != nil {
		notify.Stop(c)
		close(c)
		return nil, interpretNotifyWatchError(err, watcher.folderPath)
	}
	l.Infoln(watcher, "Started FsWatcher")
	return c, nil
}

func (watcher *FsWatcher) watchFilesystem() {
	watcher.notifyTimer = time.NewTimer(watcher.notifyDelay)
	defer watcher.notifyTimer.Stop()
	inProgressItemSubscription := events.Default.Subscribe(
		events.ItemStarted | events.ItemFinished)
	for {
		watcher.resetNotifyTimerIfNeeded()
		select {
		case event, _ := <-watcher.fsEventChan:
			watcher.newFsEvent(event.Path())
		case <-watcher.notifyTimer.C:
			watcher.actOnTimer()
		case event := <-inProgressItemSubscription.C():
			watcher.updateInProgressSet(event)
		case ignores := <-watcher.ignoresUpdate:
			watcher.ignores = ignores
		case <-watcher.stop:
			notify.Stop(watcher.fsEventChan)
			return
		}
	}
}

func (watcher *FsWatcher) Stop() {
	if watcher.WatchingFs {
		watcher.WatchingFs = false
		watcher.stop <- struct{}{}
		l.Infoln(watcher, "Stopped FsWatcher")
	} else {
		l.Debugln(watcher, "FsWatcher isn't running, nothing to stop.")
	}
}

func (watcher *FsWatcher) newFsEvent(eventPath string) {
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
	if len(path) > 1 && os.IsPathSeparator(path[len(path)-1]) {
		path = path[0 : len(path)-1]
	}
	if len(folderPath) > 1 && os.IsPathSeparator(folderPath[len(folderPath)-1]) {
		folderPath = folderPath[0 : len(folderPath)-1]
	}
	return strings.HasPrefix(path, folderPath)
}

func (watcher *FsWatcher) resetNotifyTimerIfNeeded() {
	if watcher.notifyTimerNeedsReset {
		l.Debugf("%v Resetting notifyTimer to %s", watcher,
			watcher.notifyDelay.String())
		watcher.notifyTimer.Reset(watcher.notifyDelay)
		watcher.notifyTimerNeedsReset = false
	}
}

func (watcher *FsWatcher) speedUpNotifyTimer() {
	if watcher.notifyDelay != fastNotifyDelay {
		watcher.notifyDelay = fastNotifyDelay
		l.Debugf("%v Speeding up notifyTimer to %s", watcher,
			fastNotifyDelay.String())
		watcher.notifyTimerNeedsReset = true
	}
}

func (watcher *FsWatcher) slowDownNotifyTimer() {
	if watcher.notifyDelay != watcher.slowNotifyDelay {
		watcher.notifyDelay = watcher.slowNotifyDelay
		l.Debugf("%v Slowing down notifyTimer to %s", watcher,
			watcher.notifyDelay.String())
		watcher.notifyTimerNeedsReset = true
	}
}

func (watcher *FsWatcher) aggregateEvent(path string, eventTime time.Time) {
	if path == "." || watcher.rootEventDir.eventCount() == maxFiles {
		l.Debugln(watcher, "Scan entire folder")
		watcher.rootEventDir = newEventDir(".", nil)
		watcher.rootEventDir.events["."] = &FsEvent{".", eventTime}
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

		if _, ok := parentDir.events[currPath]; ok {
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

	if _, ok := parentDir.events[path]; ok {
		l.Debugf("%v Already tracked: %s", watcher, path)
		return
	}

	_, ok := parentDir.dirs[path]

	// If a dir existed at path, it would be removed from dirs, thus
	// childCount would not increase.
	if !ok && parentDir.childCount() == localMaxFilesPerDir {
		l.Debugf("%v Parent dir already has %d children, tracking it "+
			"instead: %s", watcher, localMaxFilesPerDir, path)
		watcher.aggregateEvent(filepath.Dir(path), eventTime)
		return
	}

	if ok {
		l.Debugf("%v Removing eventDir: %s", watcher, path)
		delete(parentDir.dirs, path)
	}

	l.Debugf("%v Tracking: %s", watcher, path)
	parentDir.events[path] = &FsEvent{path, eventTime}
	watcher.resetNotifyTimerIfNeeded()
}

func (watcher *FsWatcher) actOnTimer() {
	watcher.notifyTimerNeedsReset = true
	eventCount := watcher.rootEventDir.eventCount()
	if eventCount == 0 {
		watcher.slowDownNotifyTimer()
		return
	}
	oldFsEvents := watcher.popOldEvents(watcher.rootEventDir, time.Now())
	if len(oldFsEvents) != 0 {
		l.Debugf("%v Notifying about %d fs events", watcher,
			len(oldFsEvents))
		watcher.notifyModelChan <- oldFsEvents
	}
}

func (watcher *FsWatcher) popOldEvents(dir *eventDir, currTime time.Time) FsEventsBatch {
	oldEvents := make(FsEventsBatch)
	for _, childDir := range dir.dirs {
		for path, event := range watcher.popOldEvents(childDir, currTime) {
			oldEvents[path] = event
		}
	}
	for path, event := range dir.events {
		if currTime.Sub(event.time) > fastNotifyDelay {
			oldEvents[path] = event
			delete(dir.events, path)
		}
	}
	if dir.parentDir != nil && dir.childCount() == 0 {
		dir.parentDir.removeEmptyDir(dir.path)
	}
	return oldEvents
}

func (watcher *FsWatcher) updateInProgressSet(event events.Event) {
	if event.Type == events.ItemStarted {
		path := event.Data.(map[string]string)["item"]
		watcher.inProgress[path] = struct{}{}
	} else if event.Type == events.ItemFinished {
		path := event.Data.(map[string]interface{})["item"].(string)
		delete(watcher.inProgress, path)
	}
}

func (watcher *FsWatcher) pathInProgress(path string) bool {
	_, exists := watcher.inProgress[path]
	return exists
}

func (watcher *FsWatcher) UpdateIgnores(ignores *ignore.Matcher) {
	if watcher.WatchingFs {
		watcher.ignoresUpdate <- ignores
	}
}

func (watcher *FsWatcher) String() string {
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

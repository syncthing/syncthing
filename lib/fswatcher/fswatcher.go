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

type FsWatcher struct {
	folderPath      string
	notifyModelChan chan<- FsEventsBatch
	// All detected and to be scanned events.
	fsEvents              FsEventsBatch
	fsEventChan           <-chan notify.EventInfo
	WatchingFs            bool
	notifyDelay           time.Duration
	slowNotifyDelay       time.Duration
	notifyTimer           *time.Timer
	notifyTimerNeedsReset bool
	inProgress            map[string]struct{}
	folderID              string
	ignores               *ignore.Matcher
	ignoresUpdate         chan *ignore.Matcher
	// Keeps track the events that are tracked within a directory for event
	// aggregation. The directory itself is not (yet) to be scanned.
	trackedDirs map[string]FsEventsBatch
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
		fsEvents:              make(FsEventsBatch),
		fsEventChan:           nil,
		WatchingFs:            false,
		notifyDelay:           fastNotifyDelay,
		slowNotifyDelay:       time.Duration(slowNotifyDelayS) * time.Second,
		notifyTimerNeedsReset: false,
		inProgress:            make(map[string]struct{}),
		folderID:              folderID,
		ignores:               ignores,
		ignoresUpdate:         make(chan *ignore.Matcher),
		trackedDirs:           make(map[string]FsEventsBatch),
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
		}
	}
}

func (watcher *FsWatcher) newFsEvent(eventPath string) {
	if len(watcher.fsEvents) == maxFiles {
		l.Debugf("%v Tracking too many events; dropping: %s", watcher, eventPath)
	} else if _, ok := watcher.fsEvents["."]; ok {
		l.Debugf("%v Will scan entire folder anyway; dropping: %s",
			watcher, eventPath)
	} else if isSubpath(eventPath, watcher.folderPath) {
		path, _ := filepath.Rel(watcher.folderPath, eventPath)
		if watcher.pathInProgress(path) {
			l.Debugf("%v Skipping notification for path we modified: %s",
				watcher, path)
		} else if watcher.ignores.ShouldIgnore(path) {
			l.Debugf("%v Ignoring: %s", watcher, path)
		} else {
			watcher.aggregateEvent(path, time.Now())
		}
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
	if path == "." {
		l.Debugln(watcher, "Aggregating: Scan entire folder")
		watcher.fsEvents = make(FsEventsBatch)
		watcher.fsEvents["."] = &FsEvent{".", eventTime}
		watcher.speedUpNotifyTimer()
		return
	}
	// Check if any parent directory is already tracked.
	for testPath := path; testPath != "."; testPath = filepath.Dir(testPath) {
		if _, ok := watcher.fsEvents[testPath]; ok {
			l.Debugf("%v Aggregating: Parent path already tracked: %s",
				watcher, path)
			return
		}
	}
	parentPath := filepath.Dir(path)
	// Events in the basepath cannot be aggregated -> allow up to maxFiles events
	localMaxFilesPerDir := maxFilesPerDir
	if parentPath == "." {
		localMaxFilesPerDir = maxFiles
	}
	dir, ok := watcher.trackedDirs[parentPath]
	if ok && len(dir) == localMaxFilesPerDir {
		l.Debugf("%v Aggregating: Parent dir already contains %d events,"+
			"track it instead: %s", watcher, localMaxFilesPerDir, path)
		// Keep time of oldest event, otherwise scanning may be delayed.
		for childPath, childEvent := range dir {
			if childEvent.time.Before(eventTime) {
				eventTime = childEvent.time
			}
			delete(watcher.fsEvents, childPath)
		}
		delete(watcher.trackedDirs, parentPath)
		watcher.aggregateEvent(parentPath, eventTime)
		return
	}
	if !ok {
		watcher.trackedDirs[parentPath] = make(FsEventsBatch)
	}
	watcher.fsEvents[path] = &FsEvent{path, eventTime}
	watcher.trackedDirs[parentPath][path] = watcher.fsEvents[path]
	watcher.speedUpNotifyTimer()
}

func (watcher *FsWatcher) actOnTimer() {
	watcher.notifyTimerNeedsReset = true
	if len(watcher.fsEvents) > 0 {
		watcher.notifyModelChan <- watcher.extractOldEvents()
	} else {
		watcher.slowDownNotifyTimer()
	}
}

func (watcher *FsWatcher) extractOldEvents() FsEventsBatch {
	oldFsEvents := make(FsEventsBatch)
	if len(watcher.fsEvents) == maxFiles {
		l.Debugln(watcher, "Too many changes, issuing full rescan.")
		oldFsEvents["."] = &FsEvent{".", time.Now()}
		watcher.fsEvents = make(FsEventsBatch)
		watcher.trackedDirs = make(map[string]FsEventsBatch)
	} else {
		l.Debugf("%v Notifying about %d fs events", watcher,
			len(watcher.fsEvents))
		currTime := time.Now()
		for path, event := range watcher.fsEvents {
			if currTime.Sub(event.time) > fastNotifyDelay {
				oldFsEvents[path] = event
				delete(watcher.fsEvents, path)
				parentPath := filepath.Dir(path)
				if len(watcher.trackedDirs[parentPath]) == 1 {
					delete(watcher.trackedDirs, parentPath)
				} else {
					delete(watcher.trackedDirs[parentPath], path)
				}
			}
		}
	}
	return oldFsEvents
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

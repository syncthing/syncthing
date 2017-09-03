// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package fswatcher

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/sync"
)

// Not meant to be changed, but must be changeable for tests
var (
	maxFiles       = 512
	maxFilesPerDir = 128
)

// aggregatedEvent represents potentially multiple events at and/or recursively
// below one path until it times out and a scan is scheduled.
type aggregatedEvent struct {
	firstModTime time.Time
	lastModTime  time.Time
	evType       fs.EventType
}

// Stores pointers to both aggregated events directly within this directory and
// child directories recursively containing aggregated events themselves.
type eventDir struct {
	events map[string]*aggregatedEvent
	dirs   map[string]*eventDir
}

func newEventDir() *eventDir {
	return &eventDir{
		events: make(map[string]*aggregatedEvent),
		dirs:   make(map[string]*eventDir),
	}
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

func (dir *eventDir) firstModTime() time.Time {
	if dir.childCount() == 0 {
		panic("bug: firstModTime must not be used on empty eventDir")
	}
	firstModTime := time.Now()
	for _, childDir := range dir.dirs {
		dirTime := childDir.firstModTime()
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

func (dir *eventDir) eventType() fs.EventType {
	if dir.childCount() == 0 {
		panic("bug: eventType must not be used on empty eventDir")
	}
	var evType fs.EventType
	for _, childDir := range dir.dirs {
		evType |= childDir.eventType()
		if evType == fs.Mixed {
			return fs.Mixed
		}
	}
	for _, event := range dir.events {
		evType |= event.evType
		if evType == fs.Mixed {
			return fs.Mixed
		}
	}
	return evType
}

type watcher struct {
	folderPath      string
	folderCfg       config.FolderConfiguration
	folderCfgUpdate chan config.FolderConfiguration
	ignores         *ignore.Matcher
	ignoresUpdate   chan *ignore.Matcher
	// Time after which an event is scheduled for scanning when no modifications occur.
	notifyDelay time.Duration
	// Time after which an event is scheduled for scanning even though modifications occur.
	notifyTimeout         time.Duration
	notifyTimer           *time.Timer
	notifyTimerNeedsReset bool
	notifyTimerResetChan  chan time.Duration
	notifyChan            chan []string
	cfg                   *config.Wrapper
	err                   error
	errMut                sync.RWMutex
	ctx                   context.Context
	cancel                context.CancelFunc
}

type Service interface {
	Serve()
	Stop()
	C() <-chan []string
	UpdateIgnores(ignores *ignore.Matcher)
	VerifyConfiguration(from, to config.Configuration) error
	CommitConfiguration(from, to config.Configuration) bool
	String() string
}

func New(folderCfg config.FolderConfiguration, cfg *config.Wrapper, ignores *ignore.Matcher) Service {
	ctx, cancel := context.WithCancel(context.Background())

	fsWatcher := &watcher{
		ignores:               ignores,
		ignoresUpdate:         make(chan *ignore.Matcher),
		folderCfgUpdate:       make(chan config.FolderConfiguration),
		notifyTimerNeedsReset: false,
		notifyTimerResetChan:  make(chan time.Duration),
		notifyChan:            make(chan []string),
		cfg:                   cfg,
		errMut:                sync.NewRWMutex(),
		ctx:                   ctx,
		cancel:                cancel,
	}

	fsWatcher.updateConfig(folderCfg)

	return fsWatcher
}

func (w *watcher) Serve() {
	watchCtx, watchCancel := context.WithCancel(w.ctx)
	eventChan, err := w.folderCfg.Filesystem().Watch(".", w.ignores, watchCtx, w.folderCfg.IgnorePerms)
	if err != nil {
		l.Debugln(w, "failed to setup backend", err)
		w.errMut.Lock()
		if err != w.err {
			l.Warnf("Failed to start filesystem watcher for folder %s: %v", w.folderCfg.Description(), err)
			w.err = err
		}
		w.errMut.Unlock()
		return
	}

	w.errMut.Lock()
	w.err = nil
	w.errMut.Unlock()
	l.Infoln("Started filesystem watcher for folder", w.folderCfg.Description())

	// Will not return unless watcher is stopped or an unrecoverable error occurs
	// Necessary for unit tests where the backend is mocked
	w.mainLoop(eventChan, watchCancel)
}

func (w *watcher) mainLoop(eventChan <-chan fs.Event, watchCancel context.CancelFunc) {
	w.notifyTimer = time.NewTimer(w.notifyDelay)
	defer w.notifyTimer.Stop()

	inProgress := make(map[string]struct{})
	inProgressItemSubscription := events.Default.Subscribe(events.ItemStarted | events.ItemFinished)

	w.cfg.Subscribe(w)

	rootEventDir := newEventDir()

	for {
		select {
		case event := <-eventChan:
			w.newEvent(event, rootEventDir, inProgress)
		case event := <-inProgressItemSubscription.C():
			updateInProgressSet(event, inProgress)
		case <-w.notifyTimer.C:
			w.actOnTimer(rootEventDir)
		case interval := <-w.notifyTimerResetChan:
			w.resetNotifyTimer(interval)
		case ignores := <-w.ignoresUpdate:
			watchCancel()
			w.ignores = ignores
			var err error
			var watchCtx context.Context
			watchCtx, watchCancel = context.WithCancel(w.ctx)
			eventChan, err = w.folderCfg.Filesystem().Watch(".", w.ignores, watchCtx, w.folderCfg.IgnorePerms)
			if err != nil {
				l.Warnf("Failed to setup filesystem watcher after ignore patterns changed for folder %s: %v", w.folderCfg.Description(), err)
				w.errMut.Lock()
				w.err = err
				w.errMut.Unlock()
				return
			}
		case folderCfg := <-w.folderCfgUpdate:
			w.updateConfig(folderCfg)
		case <-w.ctx.Done():
			w.cfg.Unsubscribe(w)
			l.Infoln("Stopped filesystem watcher for folder", w.folderCfg.Description())
			return
		}
	}
}

func (w *watcher) Stop() {
	w.cancel()
}

func (w *watcher) C() <-chan []string {
	return w.notifyChan
}

func (w *watcher) newEvent(event fs.Event, rootEventDir *eventDir, inProgress map[string]struct{}) {
	if _, ok := rootEventDir.events["."]; ok {
		l.Debugf("%v Will scan entire folder anyway; dropping: %s", w, event.Name)
		return
	}
	if _, ok := inProgress[event.Name]; ok {
		l.Debugf("%v Skipping path we modified: %s", w, event.Name)
		return
	}
	w.aggregateEvent(event, time.Now(), rootEventDir)
}

// Provide to be checked path first, then the path of the folder root.
var isSubpath = strings.HasPrefix

func (w *watcher) resetNotifyTimerIfNeeded() {
	if w.notifyTimerNeedsReset {
		w.resetNotifyTimer(w.notifyDelay)
	}
}

// resetNotifyTimer should only ever be called when notifyTimer has stopped
// and notifyTimer.C been read from. Otherwise, call resetNotifyTimerIfNeeded.
func (w *watcher) resetNotifyTimer(duration time.Duration) {
	l.Debugf("%v Resetting notifyTimer to %s", w, duration.String())
	w.notifyTimerNeedsReset = false
	w.notifyTimer.Reset(duration)
}

func (w *watcher) aggregateEvent(event fs.Event, evTime time.Time, rootEventDir *eventDir) {
	if event.Name == "." || rootEventDir.eventCount() == maxFiles {
		l.Debugln(w, "Scan entire folder")
		firstModTime := evTime
		if rootEventDir.childCount() != 0 {
			event.Type |= rootEventDir.eventType()
			firstModTime = rootEventDir.firstModTime()
		}
		rootEventDir.dirs = make(map[string]*eventDir)
		rootEventDir.events = make(map[string]*aggregatedEvent)
		rootEventDir.events["."] = &aggregatedEvent{
			firstModTime: firstModTime,
			lastModTime:  evTime,
			evType:       event.Type,
		}
		w.resetNotifyTimerIfNeeded()
		return
	}

	parentDir := rootEventDir

	// Check if any parent directory is already tracked or will exceed
	// events per directory limit bottom up
	pathSegments := strings.Split(filepath.ToSlash(event.Name), "/")

	// As root dir cannot be further aggregated, allow up to maxFiles
	// children.
	localMaxFilesPerDir := maxFiles
	var currPath string
	for i, name := range pathSegments[:len(pathSegments)-1] {
		currPath = filepath.Join(currPath, name)

		if ev, ok := parentDir.events[name]; ok {
			ev.lastModTime = evTime
			ev.evType |= event.Type
			l.Debugf("%v Parent %s (type %s) already tracked: %s", w, currPath, ev.evType, event.Name)
			return
		}

		if parentDir.childCount() == localMaxFilesPerDir {
			l.Debugf("%v Parent dir %s already has %d children, tracking it instead: %s", w, currPath, localMaxFilesPerDir, event.Name)
			event.Name = filepath.Dir(currPath)
			w.aggregateEvent(event, evTime, rootEventDir)
			return
		}

		// If there are no events below path, but we need to recurse
		// into that path, create eventDir at path.
		if newParent, ok := parentDir.dirs[name]; ok {
			parentDir = newParent
		} else {
			l.Debugf("%v Creating eventDir at: %s", w, currPath)
			newParent = newEventDir()
			parentDir.dirs[name] = newParent
			parentDir = newParent
		}

		// Reset allowed children count to maxFilesPerDir for non-root
		if i == 0 {
			localMaxFilesPerDir = maxFilesPerDir
		}
	}

	name := pathSegments[len(pathSegments)-1]

	if ev, ok := parentDir.events[name]; ok {
		ev.lastModTime = evTime
		ev.evType |= event.Type
		l.Debugf("%v Already tracked (type %v): %s", w, ev.evType, event.Name)
		return
	}

	childDir, ok := parentDir.dirs[name]

	// If a dir existed at path, it would be removed from dirs, thus
	// childCount would not increase.
	if !ok && parentDir.childCount() == localMaxFilesPerDir {
		l.Debugf("%v Parent dir already has %d children, tracking it instead: %s", w, localMaxFilesPerDir, event.Name)
		event.Name = filepath.Dir(event.Name)
		w.aggregateEvent(event, evTime, rootEventDir)
		return
	}

	firstModTime := evTime
	if ok {
		firstModTime = childDir.firstModTime()
		event.Type |= childDir.eventType()
		delete(parentDir.dirs, name)
	}
	l.Debugf("%v Tracking (type %v): %s", w, event.Type, event.Name)
	parentDir.events[name] = &aggregatedEvent{
		firstModTime: firstModTime,
		lastModTime:  evTime,
		evType:       event.Type,
	}
	w.resetNotifyTimerIfNeeded()
}

func (w *watcher) actOnTimer(rootEventDir *eventDir) {
	eventCount := rootEventDir.eventCount()
	if eventCount == 0 {
		l.Debugln(w, "No tracked events, waiting for new event.")
		w.notifyTimerNeedsReset = true
		return
	}
	oldevents := w.popOldEvents(rootEventDir, ".", time.Now())
	if len(oldevents) == 0 {
		l.Debugln(w, "No old fs events")
		w.resetNotifyTimer(w.notifyDelay)
		return
	}
	// Sending to channel might block for a long time, but we need to keep
	// reading from notify backend channel to avoid overflow
	go w.notify(oldevents)
}

// Schedule scan for given events dispatching deletes last and reset notification
// afterwards to set up for the next scan scheduling.
func (w *watcher) notify(oldEvents map[string]*aggregatedEvent) {
	timeBeforeSending := time.Now()
	l.Debugf("%v Notifying about %d fs events", w, len(oldEvents))
	separatedBatches := make(map[fs.EventType][]string)
	for path, event := range oldEvents {
		separatedBatches[event.evType] = append(separatedBatches[event.evType], path)
	}
	for _, evType := range [3]fs.EventType{fs.NonRemove, fs.Mixed, fs.Remove} {
		currBatch := separatedBatches[evType]
		if len(currBatch) != 0 {
			select {
			case w.notifyChan <- currBatch:
			case <-w.ctx.Done():
				return
			}
		}
	}
	// If sending to channel blocked for a long time,
	// shorten next notifyDelay accordingly.
	duration := time.Since(timeBeforeSending)
	buffer := time.Millisecond
	var nextDelay time.Duration
	switch {
	case duration < w.notifyDelay/10:
		nextDelay = w.notifyDelay
	case duration+buffer > w.notifyDelay:
		nextDelay = buffer
	default:
		nextDelay = w.notifyDelay - duration
	}
	select {
	case w.notifyTimerResetChan <- nextDelay:
	case <-w.ctx.Done():
	}
}

// popOldEvents finds events that should be scheduled for scanning recursively in dirs,
// removes those events and empty eventDirs and returns a map with all the removed
// events referenced by their filesystem path
func (w *watcher) popOldEvents(dir *eventDir, dirPath string, currTime time.Time) map[string]*aggregatedEvent {
	oldEvents := make(map[string]*aggregatedEvent)
	for childName, childDir := range dir.dirs {
		for evPath, event := range w.popOldEvents(childDir, filepath.Join(dirPath, childName), currTime) {
			oldEvents[evPath] = event
		}
		if childDir.childCount() == 0 {
			delete(dir.dirs, childName)
		}
	}
	for name, event := range dir.events {
		if w.isOld(event, currTime) {
			oldEvents[filepath.Join(dirPath, name)] = event
			delete(dir.events, name)
		}
	}
	return oldEvents
}

func (w *watcher) isOld(ev *aggregatedEvent, currTime time.Time) bool {
	// Deletes should always be scanned last, therefore they are always
	// delayed by letting them time out (see below).
	// An event that has not registered any new modifications recently is scanned.
	// w.notifyDelay is the user facing value signifying the normal delay between
	// a picking up a modification and scanning it. As scheduling scans happens at
	// regular intervals of w.notifyDelay the delay of a single event is not exactly
	// w.notifyDelay, but lies in in the range of 0.5 to 1.5 times w.notifyDelay.
	if ev.evType == fs.NonRemove && 2*currTime.Sub(ev.lastModTime) > w.notifyDelay {
		return true
	}
	// When an event registers repeat modifications or involves removals it
	// is delayed to reduce resource usage, but after a certain time (notifyTimeout)
	// passed it is scanned anyway.
	return currTime.Sub(ev.firstModTime) > w.notifyTimeout
}

func (w *watcher) UpdateIgnores(ignores *ignore.Matcher) {
	l.Debugln(w, "Ignore patterns update")
	w.errMut.RLock()
	defer w.errMut.RUnlock()
	if w.err != nil {
		return
	}
	select {
	case w.ignoresUpdate <- ignores:
	case <-w.ctx.Done():
	}
}

func (w *watcher) String() string {
	return fmt.Sprintf("fswatcher/%s:", w.folderCfg.Description())
}

func (w *watcher) VerifyConfiguration(from, to config.Configuration) error {
	return nil
}

func (w *watcher) CommitConfiguration(from, to config.Configuration) bool {
	w.errMut.RLock()
	defer w.errMut.RUnlock()
	if w.err != nil {
		return true
	}
	for _, folderCfg := range to.Folders {
		if folderCfg.ID == w.folderCfg.ID {
			select {
			case w.folderCfgUpdate <- folderCfg:
			case <-w.ctx.Done():
			}
			return true
		}
	}
	// Nothing to do, model will soon stop this service
	return true
}

func (w *watcher) updateConfig(folderCfg config.FolderConfiguration) {
	w.notifyDelay = time.Duration(folderCfg.FSWatcherDelayS) * time.Second
	w.notifyTimeout = notifyTimeout(folderCfg.FSWatcherDelayS)
	w.folderCfg = folderCfg
}

func updateInProgressSet(event events.Event, inProgress map[string]struct{}) {
	if event.Type == events.ItemStarted {
		path := event.Data.(map[string]string)["item"]
		inProgress[path] = struct{}{}
	} else if event.Type == events.ItemFinished {
		path := event.Data.(map[string]interface{})["item"].(string)
		delete(inProgress, path)
	}
}

// Events that involve removals or continuously receive new modifications are
// delayed but must time out at some point. The following numbers come out of thin
// air, they were just considered as a sensible compromise between fast updates and
// saving resources. For short delays the timeout is 6 times the delay, capped at 1
// minute. For delays longer than 1 minute, the delay and timeout are equal.
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

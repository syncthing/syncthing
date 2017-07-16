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

type eventType int

const (
	nonRemove eventType = 1
	remove              = 2
	mixed               = 3
)

// Not meant to be changed, but must be changeable for tests
var (
	maxFiles       = 512
	maxFilesPerDir = 128
)

// event represents detected changes at one path until it times out and a scan is scheduled
type event struct {
	path         string
	firstModTime time.Time
	lastModTime  time.Time
	evType       eventType
}

type eventBatch map[string]*event

// Stores both events directly within this directory (events) and child
// directories recursively containing events themselves (dirs). The keys to the
// events and dirs maps are the full relative path to the Syncthing folder root.
type eventDir struct {
	path      string
	parentDir *eventDir
	events    eventBatch
	dirs      map[string]*eventDir
}

func newEventDir(path string, parentDir *eventDir) *eventDir {
	return &eventDir{
		path:      path,
		parentDir: parentDir,
		events:    make(eventBatch),
		dirs:      make(map[string]*eventDir),
	}
}

type watcher struct {
	folderID            string
	folderPath          string
	folderIgnores       *ignore.Matcher
	folderIgnoresUpdate chan *ignore.Matcher
	folderCfg           config.FolderConfiguration
	// Time after which an event is scheduled for scanning when no modifications occur.
	notifyDelay time.Duration
	// Time after which an event is scheduled for scanning even though modifications occur.
	notifyTimeout         time.Duration
	notifyTimer           *time.Timer
	notifyTimerNeedsReset bool
	notifyTimerResetChan  chan time.Duration
	notifyChan            chan []string
	// All detected and to be scanned events are stored in a tree like
	// structure mimicking directories to keep count of events per directory.
	rootEventDir     *eventDir
	backendEventChan chan notify.EventInfo
	// paths currently processed by Syncthing (i.e. ignore modifications on them)
	inProgress   map[string]struct{}
	cfg          *config.Wrapper
	configUpdate chan config.FolderConfiguration
	stop         chan struct{}
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

func New(id string, cfg *config.Wrapper, ignores *ignore.Matcher) (Service, error) {
	fsWatcher := &watcher{
		folderID:              id,
		folderIgnores:         ignores,
		folderIgnoresUpdate:   make(chan *ignore.Matcher),
		notifyTimerNeedsReset: false,
		notifyTimerResetChan:  make(chan time.Duration),
		notifyChan:            make(chan []string),
		rootEventDir:          newEventDir(".", nil),
		backendEventChan:      make(chan notify.EventInfo, maxFiles),
		inProgress:            make(map[string]struct{}),
		cfg:                   cfg,
		configUpdate:          make(chan config.FolderConfiguration),
		stop:                  make(chan struct{}),
	}
	folderCfg, ok := cfg.Folder(id)
	if !ok {
		panic(fmt.Sprintf("bug: Folder %s does not exist", id))
	}
	fsWatcher.updateConfig(folderCfg)

	if err := fsWatcher.setupBackend(); err != nil {
		return nil, err
	}

	return fsWatcher, nil
}

func (w *watcher) setupBackend() error {
	absShouldIgnore := func(absPath string) bool {
		if !isSubpath(absPath, w.folderPath) {
			panic(fmt.Sprintf("bug: Notify backend is processing a change outside of the root dir of folder %v", w.folderCfg.Description()))
		}
		relPath, _ := filepath.Rel(w.folderPath, absPath)
		return w.folderIgnores.ShouldIgnore(relPath)
	}
	if err := notify.WatchWithFilter(filepath.Join(w.folderPath, "..."), w.backendEventChan, absShouldIgnore, w.eventMask()); err != nil {
		notify.Stop(w.backendEventChan)
		close(w.backendEventChan)
		if isOutOfFileHandles(err) {
			err = watchesLimitTooLowError(w.folderCfg.Description())
		}
		return err
	}
	return nil
}

func (w *watcher) Serve() {
	w.notifyTimer = time.NewTimer(w.notifyDelay)
	defer w.notifyTimer.Stop()

	inProgressItemSubscription := events.Default.Subscribe(events.ItemStarted | events.ItemFinished)

	w.cfg.Subscribe(w)

	for {
		// Detect channel overflow
		if len(w.backendEventChan) == maxFiles {
		outer:
			for {
				select {
				case <-w.backendEventChan:
				default:
					break outer
				}
			}
			// When next scheduling a scan, do it on the entire folder as events have been lost.
			w.newEvent(w.folderPath, nonRemove)
			l.Debugln(w, "Backend channel overflow: Scan entire folder")
		}
		select {
		case event, _ := <-w.backendEventChan:
			w.newEvent(event.Path(), w.eventType(event.Event()))
		case event := <-inProgressItemSubscription.C():
			w.updateInProgressSet(event)
		case <-w.notifyTimer.C:
			w.actOnTimer()
		case interval := <-w.notifyTimerResetChan:
			w.resetNotifyTimer(interval)
		case ignores := <-w.folderIgnoresUpdate:
			w.folderIgnores = ignores
		case folderCfg := <-w.configUpdate:
			w.updateConfig(folderCfg)
		case <-w.stop:
			return
		}
	}
}

func (w *watcher) Stop() {
	close(w.stop)
	notify.Stop(w.backendEventChan)
	w.cfg.Unsubscribe(w)
	l.Infoln("Stopped filesystem watcher for folder", w.folderCfg.Description())
}

func (w *watcher) C() <-chan []string {
	return w.notifyChan
}

func (w *watcher) newEvent(evPath string, evType eventType) {
	if _, ok := w.rootEventDir.events["."]; ok {
		l.Debugf("%v Will scan entire folder anyway; dropping: %s", w, evPath)
		return
	}
	if !isSubpath(evPath, w.folderPath) {
		panic(fmt.Sprintf("bug: Detected change outside of root directory for folder %v at %s", w.folderCfg.Description(), evPath))
	}
	relPath, _ := filepath.Rel(w.folderPath, evPath)
	if w.pathInProgress(relPath) {
		l.Debugf("%v Skipping path we modified: %s", w, relPath)
		return
	}
	if w.folderIgnores.ShouldIgnore(relPath) {
		l.Debugf("%v Ignoring: %s", w, relPath)
		return
	}
	w.aggregateEvent(relPath, time.Now(), evType)
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

func (w *watcher) aggregateEvent(evPath string, evTime time.Time, evType eventType) {
	if evPath == "." || w.rootEventDir.eventCount() == maxFiles {
		l.Debugln(w, "Scan entire folder")
		firstModTime := evTime
		if w.rootEventDir.childCount() != 0 {
			evType |= w.rootEventDir.eventType()
			firstModTime = w.rootEventDir.firstModTime()
		}
		w.rootEventDir = newEventDir(".", nil)
		w.rootEventDir.events["."] = &event{
			path:         ".",
			firstModTime: firstModTime,
			lastModTime:  evTime,
			evType:       evType,
		}
		w.resetNotifyTimerIfNeeded()
		return
	}

	parentDir := w.rootEventDir

	// Check if any parent directory is already tracked or will exceed
	// events per directory limit bottom up
	pathSegments := strings.Split(filepath.ToSlash(evPath), "/")

	// As root dir cannot be further aggregated, allow up to maxFiles
	// children.
	localMaxFilesPerDir := maxFiles
	var currPath string
	for i, pathSegment := range pathSegments[:len(pathSegments)-1] {
		currPath = filepath.Join(currPath, pathSegment)

		if event, ok := parentDir.events[currPath]; ok {
			event.lastModTime = evTime
			event.evType |= evType
			l.Debugf("%v Parent %s (type %s) already tracked: %s", w, currPath, event.evType, evPath)
			return
		}

		if parentDir.childCount() == localMaxFilesPerDir {
			l.Debugf("%v Parent dir %s already has %d children, tracking it instead: %s", w, currPath, localMaxFilesPerDir, evPath)
			w.aggregateEvent(filepath.Dir(currPath),
				evTime, evType)
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

	if event, ok := parentDir.events[evPath]; ok {
		event.lastModTime = evTime
		event.evType |= evType
		l.Debugf("%v Already tracked (type %v): %s", w, event.evType, evPath)
		return
	}

	childDir, ok := parentDir.dirs[evPath]

	// If a dir existed at path, it would be removed from dirs, thus
	// childCount would not increase.
	if !ok && parentDir.childCount() == localMaxFilesPerDir {
		l.Debugf("%v Parent dir already has %d children, tracking it instead: %s", w, localMaxFilesPerDir, evPath)
		w.aggregateEvent(filepath.Dir(evPath), evTime, evType)
		return
	}

	firstModTime := evTime
	if ok {
		firstModTime = childDir.firstModTime()
		evType |= childDir.eventType()
		delete(parentDir.dirs, evPath)
	}
	l.Debugf("%v Tracking (type %v): %s", w, evType, evPath)
	parentDir.events[evPath] = &event{
		path:         evPath,
		firstModTime: firstModTime,
		lastModTime:  evTime,
		evType:       evType,
	}
	w.resetNotifyTimerIfNeeded()
}

func (w *watcher) actOnTimer() {
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
	go w.notify(oldFsEvents)
}

// Schedule scan for given events dispatching deletes last and reset notification
// afterwards to set up for the next scan scheduling.
func (w *watcher) notify(oldFsEvents eventBatch) {
	timeBeforeSending := time.Now()
	l.Debugf("%v Notifying about %d fs events", w, len(oldFsEvents))
	separatedBatches := make(map[eventType][]string)
	for path, event := range oldFsEvents {
		separatedBatches[event.evType] = append(separatedBatches[event.evType], path)
	}
	for _, evType := range [3]eventType{nonRemove, mixed, remove} {
		currBatch := separatedBatches[evType]
		if len(currBatch) != 0 {
			select {
			case w.notifyChan <- currBatch:
			case <-w.stop:
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
	case <-w.stop:
	}
}

// popOldEvents finds events that should be scheduled for scanning recursively in dirs,
// removes those events and empty eventDirs and returns all these events in an eventBatch.
func (w *watcher) popOldEvents(dir *eventDir, currTime time.Time) eventBatch {
	oldEvents := make(eventBatch)
	for _, childDir := range dir.dirs {
		for path, event := range w.popOldEvents(childDir, currTime) {
			oldEvents[path] = event
		}
	}
	for path, event := range dir.events {
		if w.isOld(event, currTime) {
			oldEvents[path] = event
			delete(dir.events, path)
		}
	}
	if dir.parentDir != nil && dir.childCount() == 0 {
		dir.parentDir.removeEmptyDir(dir.path)
	}
	return oldEvents
}

func (w *watcher) isOld(ev *event, currTime time.Time) bool {
	// Deletes should always be scanned last, therefore they are always
	// delayed by letting them time out.
	// An event that has not registered any new modifications recently is scanned.
	// w.notifyDelay is the user facing value signifying the normal delay between
	// a picking up a modification and scanning it. As scheduling scans happens at
	// regular intervals of w.notifyDelay the delay of a single event is not exactly
	// w.notifyDelay, but lies in in the range of 0.5 to 1.5 times w.notifyDelay.
	if ev.evType != nonRemove && 2*currTime.Sub(ev.lastModTime) > w.notifyDelay {
		return true
	}
	// When an event registers repeat modifications or involves removals it
	// is delayed to reduce resource usage, but after a certain time (notifyTimeout)
	// passed it is scanned anyway.
	return currTime.Sub(ev.firstModTime) > w.notifyTimeout
}

func (w *watcher) updateInProgressSet(event events.Event) {
	if event.Type == events.ItemStarted {
		path := event.Data.(map[string]string)["item"]
		w.inProgress[path] = struct{}{}
	} else if event.Type == events.ItemFinished {
		path := event.Data.(map[string]interface{})["item"].(string)
		delete(w.inProgress, path)
	}
}

func (w *watcher) pathInProgress(path string) bool {
	_, exists := w.inProgress[path]
	return exists
}

func (w *watcher) UpdateIgnores(ignores *ignore.Matcher) {
	l.Debugln(w, "Ignore patterns update")
	select {
	case w.folderIgnoresUpdate <- ignores:
	case <-w.stop:
	}
}

func (w *watcher) String() string {
	return fmt.Sprintf("fswatcher/%s:", w.folderCfg.Description())
}

func (w *watcher) eventType(notifyType notify.Event) eventType {
	if notifyType&w.removeEventMask() != 0 {
		return remove
	}
	return nonRemove
}

func (w *watcher) VerifyConfiguration(from, to config.Configuration) error {
	return nil
}

func (w *watcher) CommitConfiguration(from, to config.Configuration) bool {
	for _, folderCfg := range to.Folders {
		if folderCfg.ID == w.folderID {
			select {
			case w.configUpdate <- folderCfg:
			case <-w.stop:
			}
			return true
		}
	}
	// Nothing to do, model will soon stop this service
	return true
}

func (w *watcher) updateConfig(folderCfg config.FolderConfiguration) {
	w.folderPath = filepath.Clean(folderCfg.Path())
	w.notifyDelay = time.Duration(folderCfg.FSWatcherDelayS) * time.Second
	w.notifyTimeout = notifyTimeout(folderCfg.FSWatcherDelayS)
	w.folderCfg = folderCfg
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

func (dir eventDir) firstModTime() time.Time {
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

func (dir eventDir) eventType() eventType {
	if dir.childCount() == 0 {
		panic("bug: eventType must not be used on empty eventDir")
	}
	var evType eventType
	for _, childDir := range dir.dirs {
		evType |= childDir.eventType()
		if evType == mixed {
			return mixed
		}
	}
	for _, event := range dir.events {
		evType |= event.evType
		if evType == mixed {
			return mixed
		}
	}
	return evType
}

func (evType eventType) String() string {
	switch {
	case evType == nonRemove:
		return "non-remove"
	case evType == remove:
		return "remove"
	case evType == mixed:
		return "mixed"
	default:
		panic("bug: Unknown event type")
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

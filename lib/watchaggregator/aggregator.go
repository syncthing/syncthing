// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package watchaggregator

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
)

// Not meant to be changed, but must be changeable for tests
var (
	maxFiles       = 512
	maxFilesPerDir = 128
)

// aggregatedEvent represents potentially multiple events at and/or recursively
// below one path until it times out and a scan is scheduled.
// If it represents multiple events and there are events of both Remove and
// NonRemove types, the evType attribute is Mixed (as returned by fs.Event.Merge).
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

type aggregator struct {
	// folderID never changes and is accessed in CommitConfiguration, which
	// asynchronously updates folderCfg -> can't use folderCfg.ID (racy)
	folderID        string
	folderCfg       config.FolderConfiguration
	folderCfgUpdate chan config.FolderConfiguration
	// Time after which an event is scheduled for scanning when no modifications occur.
	notifyDelay time.Duration
	// Time after which an event is scheduled for scanning even though modifications occur.
	notifyTimeout         time.Duration
	notifyTimer           *time.Timer
	notifyTimerNeedsReset bool
	notifyTimerResetChan  chan time.Duration
	counts                eventCounter
	root                  *eventDir
	ctx                   context.Context
}

type eventCounter struct {
	removes    int // Includes mixed events.
	nonRemoves int
}

func (c *eventCounter) add(typ fs.EventType, n int) {
	if typ&fs.Remove != 0 {
		c.removes += n
	} else {
		c.nonRemoves += n
	}
}

func (c *eventCounter) total() int { return c.removes + c.nonRemoves }

func newAggregator(ctx context.Context, folderCfg config.FolderConfiguration) *aggregator {
	a := &aggregator{
		folderID:              folderCfg.ID,
		folderCfgUpdate:       make(chan config.FolderConfiguration),
		notifyTimerNeedsReset: false,
		notifyTimerResetChan:  make(chan time.Duration),
		root:                  newEventDir(),
		ctx:                   ctx,
	}

	a.updateConfig(folderCfg)

	return a
}

func Aggregate(ctx context.Context, in <-chan fs.Event, out chan<- []string, folderCfg config.FolderConfiguration, cfg config.Wrapper, evLogger events.Logger) {
	a := newAggregator(ctx, folderCfg)

	// Necessary for unit tests where the backend is mocked
	go a.mainLoop(in, out, cfg, evLogger)
}

func (a *aggregator) mainLoop(in <-chan fs.Event, out chan<- []string, cfg config.Wrapper, evLogger events.Logger) {
	a.notifyTimer = time.NewTimer(a.notifyDelay)
	defer a.notifyTimer.Stop()

	inProgressItemSubscription := evLogger.Subscribe(events.ItemStarted | events.ItemFinished)
	defer inProgressItemSubscription.Unsubscribe()

	cfg.Subscribe(a)
	defer cfg.Unsubscribe(a)

	inProgress := make(map[string]struct{})

	for {
		select {
		case event := <-in:
			a.newEvent(event, inProgress)
		case event, ok := <-inProgressItemSubscription.C():
			if ok {
				updateInProgressSet(event, inProgress)
			}
		case <-a.notifyTimer.C:
			a.actOnTimer(out)
		case interval := <-a.notifyTimerResetChan:
			a.resetNotifyTimer(interval)
		case folderCfg := <-a.folderCfgUpdate:
			a.updateConfig(folderCfg)
		case <-a.ctx.Done():
			l.Debugln(a, "Stopped")
			return
		}
	}
}

func (a *aggregator) newEvent(event fs.Event, inProgress map[string]struct{}) {
	if _, ok := a.root.events["."]; ok {
		l.Debugln(a, "Will scan entire folder anyway; dropping:", event.Name)
		return
	}
	if _, ok := inProgress[event.Name]; ok {
		l.Debugln(a, "Skipping path we modified:", event.Name)
		return
	}
	a.aggregateEvent(event, time.Now())
}

func (a *aggregator) aggregateEvent(event fs.Event, evTime time.Time) {
	if event.Name == "." || a.counts.total() == maxFiles {
		l.Debugln(a, "Scan entire folder")
		firstModTime := evTime
		if a.root.childCount() != 0 {
			event.Type = event.Type.Merge(a.root.eventType())
			firstModTime = a.root.firstModTime()
		}
		a.root.dirs = make(map[string]*eventDir)
		a.root.events = make(map[string]*aggregatedEvent)
		a.root.events["."] = &aggregatedEvent{
			firstModTime: firstModTime,
			lastModTime:  evTime,
			evType:       event.Type,
		}
		a.counts = eventCounter{}
		a.counts.add(event.Type, 1)
		a.resetNotifyTimerIfNeeded()
		return
	}

	parentDir := a.root

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
			if merged := event.Type.Merge(ev.evType); ev.evType != merged {
				a.counts.add(ev.evType, -1)
				a.counts.add(merged, 1)
				ev.evType = merged
			}
			l.Debugf("%v Parent %s (type %s) already tracked: %s", a, currPath, ev.evType, event.Name)
			return
		}

		if parentDir.childCount() == localMaxFilesPerDir {
			l.Debugf("%v Parent dir %s already has %d children, tracking it instead: %s", a, currPath, localMaxFilesPerDir, event.Name)
			event.Name = filepath.Dir(currPath)
			a.aggregateEvent(event, evTime)
			return
		}

		// If there are no events below path, but we need to recurse
		// into that path, create eventDir at path.
		if newParent, ok := parentDir.dirs[name]; ok {
			parentDir = newParent
		} else {
			l.Debugln(a, "Creating eventDir at:", currPath)
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
		if merged := event.Type.Merge(ev.evType); ev.evType != merged {
			a.counts.add(ev.evType, -1)
			a.counts.add(merged, 1)
			ev.evType = merged
		}
		l.Debugf("%v Already tracked (type %v): %s", a, ev.evType, event.Name)
		return
	}

	childDir, ok := parentDir.dirs[name]

	// If a dir existed at path, it would be removed from dirs, thus
	// childCount would not increase.
	if !ok && parentDir.childCount() == localMaxFilesPerDir {
		l.Debugf("%v Parent dir already has %d children, tracking it instead: %s", a, localMaxFilesPerDir, event.Name)
		event.Name = filepath.Dir(event.Name)
		a.aggregateEvent(event, evTime)
		return
	}

	firstModTime := evTime
	if ok {
		firstModTime = childDir.firstModTime()
		if merged := event.Type.Merge(childDir.eventType()); event.Type != merged {
			a.counts.add(event.Type, -1)
			event.Type = merged
		}
		delete(parentDir.dirs, name)
	}
	l.Debugf("%v Tracking (type %v): %s", a, event.Type, event.Name)
	parentDir.events[name] = &aggregatedEvent{
		firstModTime: firstModTime,
		lastModTime:  evTime,
		evType:       event.Type,
	}
	a.counts.add(event.Type, 1)
	a.resetNotifyTimerIfNeeded()
}

func (a *aggregator) resetNotifyTimerIfNeeded() {
	if a.notifyTimerNeedsReset {
		a.resetNotifyTimer(a.notifyDelay)
	}
}

// resetNotifyTimer should only ever be called when notifyTimer has stopped
// and notifyTimer.C been read from. Otherwise, call resetNotifyTimerIfNeeded.
func (a *aggregator) resetNotifyTimer(duration time.Duration) {
	l.Debugln(a, "Resetting notifyTimer to", duration.String())
	a.notifyTimerNeedsReset = false
	a.notifyTimer.Reset(duration)
}

func (a *aggregator) actOnTimer(out chan<- []string) {
	c := a.counts.total()
	if c == 0 {
		l.Debugln(a, "No tracked events, waiting for new event.")
		a.notifyTimerNeedsReset = true
		return
	}
	oldEvents := make(map[string]*aggregatedEvent, c)
	a.popOldEventsTo(oldEvents, a.root, ".", time.Now(), true)
	if a.notifyDelay != a.notifyTimeout && a.counts.nonRemoves == 0 && a.counts.removes != 0 {
		// Only delayed events remaining, no need to delay them additionally
		a.popOldEventsTo(oldEvents, a.root, ".", time.Now(), false)
	}
	if len(oldEvents) == 0 {
		l.Debugln(a, "No old fs events")
		a.resetNotifyTimer(a.notifyDelay)
		return
	}
	// Sending to channel might block for a long time, but we need to keep
	// reading from notify backend channel to avoid overflow
	go a.notify(oldEvents, out)
}

// Schedule scan for given events dispatching deletes last and reset notification
// afterwards to set up for the next scan scheduling.
func (a *aggregator) notify(oldEvents map[string]*aggregatedEvent, out chan<- []string) {
	timeBeforeSending := time.Now()
	l.Debugf("%v Notifying about %d fs events", a, len(oldEvents))
	separatedBatches := make(map[fs.EventType][]string)
	for path, event := range oldEvents {
		separatedBatches[event.evType] = append(separatedBatches[event.evType], path)
	}
	for _, evType := range [3]fs.EventType{fs.NonRemove, fs.Mixed, fs.Remove} {
		currBatch := separatedBatches[evType]
		if len(currBatch) != 0 {
			select {
			case out <- currBatch:
			case <-a.ctx.Done():
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
	case duration < a.notifyDelay/10:
		nextDelay = a.notifyDelay
	case duration+buffer > a.notifyDelay:
		nextDelay = buffer
	default:
		nextDelay = a.notifyDelay - duration
	}
	select {
	case a.notifyTimerResetChan <- nextDelay:
	case <-a.ctx.Done():
	}
}

// popOldEvents finds events that should be scheduled for scanning recursively in dirs,
// removes those events and empty eventDirs and returns a map with all the removed
// events referenced by their filesystem path
func (a *aggregator) popOldEventsTo(to map[string]*aggregatedEvent, dir *eventDir, dirPath string, currTime time.Time, delayRem bool) {
	for childName, childDir := range dir.dirs {
		a.popOldEventsTo(to, childDir, filepath.Join(dirPath, childName), currTime, delayRem)
		if childDir.childCount() == 0 {
			delete(dir.dirs, childName)
		}
	}
	for name, event := range dir.events {
		if a.isOld(event, currTime, delayRem) {
			to[filepath.Join(dirPath, name)] = event
			delete(dir.events, name)
			a.counts.add(event.evType, -1)
		}
	}
}

func (a *aggregator) isOld(ev *aggregatedEvent, currTime time.Time, delayRem bool) bool {
	// Deletes should in general be scanned last, therefore they are delayed by
	// letting them time out. This behaviour is overridden by delayRem == false.
	// Refer to following comments as to why.
	// An event that has not registered any new modifications recently is scanned.
	// a.notifyDelay is the user facing value signifying the normal delay between
	// picking up a modification and scanning it. As scheduling scans happens at
	// regular intervals of a.notifyDelay the delay of a single event is not exactly
	// a.notifyDelay, but lies in the range of 0.5 to 1.5 times a.notifyDelay.
	if (!delayRem || ev.evType == fs.NonRemove) && 2*currTime.Sub(ev.lastModTime) > a.notifyDelay {
		return true
	}
	// When an event registers repeat modifications or involves removals it
	// is delayed to reduce resource usage, but after a certain time (notifyTimeout)
	// passed it is scanned anyway.
	// If only removals are remaining to be scanned, there is no point to delay
	// removals further, so this behaviour is overridden by delayRem == false.
	return currTime.Sub(ev.firstModTime) > a.notifyTimeout
}

func (a *aggregator) String() string {
	return fmt.Sprintf("aggregator/%s:", a.folderCfg.Description())
}

func (a *aggregator) CommitConfiguration(_, to config.Configuration) bool {
	for _, folderCfg := range to.Folders {
		if folderCfg.ID == a.folderID {
			select {
			case a.folderCfgUpdate <- folderCfg:
			case <-a.ctx.Done():
			}
			return true
		}
	}
	// Nothing to do, model will soon stop this
	return true
}

func (a *aggregator) updateConfig(folderCfg config.FolderConfiguration) {
	a.notifyDelay = time.Duration(folderCfg.FSWatcherDelayS) * time.Second
	if maxDelay := folderCfg.FSWatcherMaxDelayS; maxDelay > 0 {
		// FSWatcherMaxDelayS is set explicitly so use that, but it also
		// can't be lower than FSWatcherDelayS
		a.notifyTimeout = time.Duration(max(maxDelay, folderCfg.FSWatcherDelayS)) * time.Second
	} else {
		// Use the default FSWatcherMaxDelayS calculation
		a.notifyTimeout = notifyTimeout(folderCfg.FSWatcherDelayS)
	}
	a.folderCfg = folderCfg
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
func notifyTimeout(eventDelayS float64) time.Duration {
	const (
		shortDelayS             = 10
		shortDelayMultiplicator = 6
		longDelayS              = 60
		longDelayTimeout        = time.Minute
	)
	if eventDelayS < shortDelayS {
		return time.Duration(eventDelayS*shortDelayMultiplicator) * time.Second
	}
	if eventDelayS < longDelayS {
		return longDelayTimeout
	}
	return time.Duration(eventDelayS) * time.Second
}

// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build !(solaris && !cgo) && !(darwin && !cgo) && !(darwin && kqueue) && !(android && amd64)
// +build !solaris cgo
// +build !darwin cgo
// +build !darwin !kqueue
// +build !android !amd64

package fs

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/syncthing/notify"
	"github.com/syncthing/syncthing/lib/build"
)

// Notify does not block on sending to channel, so the channel must be buffered.
// The actual number is magic.
// Not meant to be changed, but must be changeable for tests
var backendBuffer = 500

// For Windows systems with large filesets, we use a larger buffer to prevent
// event overflow which can cause file change notifications to be missed.
func init() {
	if build.IsWindows {
		// Use a larger buffer on Windows to handle large filesets better
		backendBuffer = 2000
	}
}

// overflowTracker keeps track of buffer overflow events for adaptive management
type overflowTracker struct {
	mu             sync.Mutex
	count          int
	lastOverflow   time.Time
	frequency      time.Duration
	adaptiveBuffer int
}

// newOverflowTracker creates a new overflow tracker
func newOverflowTracker() *overflowTracker {
	return &overflowTracker{
		count:          0,
		lastOverflow:   time.Time{},
		frequency:      0,
		adaptiveBuffer: backendBuffer,
	}
}

// recordOverflow records an overflow event and updates frequency tracking
func (ot *overflowTracker) recordOverflow() {
	ot.mu.Lock()
	defer ot.mu.Unlock()

	now := time.Now()
	ot.count++

	if !ot.lastOverflow.IsZero() {
		// Calculate the time between overflows
		ot.frequency = now.Sub(ot.lastOverflow)
	}

	ot.lastOverflow = now
}

// shouldIncreaseBuffer determines if we should increase the buffer size based on overflow patterns
func (ot *overflowTracker) shouldIncreaseBuffer() bool {
	ot.mu.Lock()
	defer ot.mu.Unlock()

	// If we have frequent overflows (less than 30 seconds between them) and we haven't maxed out the buffer
	return ot.frequency > 0 && ot.frequency < 30*time.Second && ot.adaptiveBuffer < 10000
}

// increaseBuffer increases the buffer size and returns the new size
func (ot *overflowTracker) increaseBuffer() int {
	ot.mu.Lock()
	defer ot.mu.Unlock()

	// Increase buffer by 50%
	ot.adaptiveBuffer = int(float64(ot.adaptiveBuffer) * 1.5)
	if ot.adaptiveBuffer > 10000 {
		ot.adaptiveBuffer = 10000 // Cap at 10000
	}

	return ot.adaptiveBuffer
}

// watchMetrics tracks performance metrics for file watching
type watchMetrics struct {
	mu              sync.Mutex
	eventsProcessed int64
	eventsDropped   int64
	overflows       int64
	startTime       time.Time
	lastEvent       time.Time
}

// newWatchMetrics creates a new watch metrics tracker
func newWatchMetrics() *watchMetrics {
	return &watchMetrics{
		startTime: time.Now(),
	}
}

// recordEvent records that an event was processed
func (wm *watchMetrics) recordEvent() {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.eventsProcessed++
	wm.lastEvent = time.Now()
}

// recordDroppedEvent records that an event was dropped
func (wm *watchMetrics) recordDroppedEvent() {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.eventsDropped++
}

// recordOverflow records a buffer overflow
func (wm *watchMetrics) recordOverflow() {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.overflows++
}

// getMetrics returns current metrics
func (wm *watchMetrics) getMetrics() (eventsProcessed, eventsDropped, overflows int64, uptime, timeSinceLastEvent time.Duration) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	now := time.Now()
	uptime = now.Sub(wm.startTime)
	timeSinceLastEvent = now.Sub(wm.lastEvent)

	return wm.eventsProcessed, wm.eventsDropped, wm.overflows, uptime, timeSinceLastEvent
}

// logMetrics periodically logs metrics for monitoring
func (wm *watchMetrics) logMetrics(fs *BasicFilesystem, name string) {
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		for range ticker.C {
			eventsProcessed, eventsDropped, overflows, uptime, timeSinceLastEvent := wm.getMetrics()
			l.Debugln(fs.Type(), fs.URI(), "Watch metrics for", name, "- Processed:", eventsProcessed,
				"Dropped:", eventsDropped, "Overflows:", overflows,
				"Uptime:", uptime.Truncate(time.Second),
				"Idle:", timeSinceLastEvent.Truncate(time.Second))
		}
	}()
}

// countFilesInDirectory counts the number of files in a directory recursively
func countFilesInDirectory(fs *BasicFilesystem, dir string) (int, error) {
	count := 0
	err := fs.Walk(dir, func(path string, info FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})
	return count, err
}

// checkLargeFolder analyzes a folder and provides recommendations if it's large
func checkLargeFolder(fs *BasicFilesystem, name string) {
	// Count files in the folder
	fileCount, err := countFilesInDirectory(fs, name)
	if err != nil {
		l.Debugln(fs.Type(), fs.URI(), "Watch: Could not count files in", name, "-", err)
		return
	}

	// If the folder has many files, provide recommendations
	if fileCount > 10000 {
		l.Debugln(fs.Type(), fs.URI(), "Watch: Folder", name, "contains", fileCount, "files which may cause performance issues.",
			"Consider excluding temporary files, build artifacts, or using more specific folder paths.")
	} else if fileCount > 5000 {
		l.Debugln(fs.Type(), fs.URI(), "Watch: Folder", name, "contains", fileCount, "files.",
			"Monitor performance and consider exclusions if issues occur.")
	} else if fileCount > 1000 {
		l.Debugln(fs.Type(), fs.URI(), "Watch: Folder", name, "contains", fileCount, "files.")
	}
}

func (f *BasicFilesystem) Watch(name string, ignore Matcher, ctx context.Context, ignorePerms bool) (<-chan Event, <-chan error, error) {
	watchPath, roots, err := f.watchPaths(name)
	if err != nil {
		return nil, nil, err
	}

	// Proactively check if this is a large folder and provide recommendations
	checkLargeFolder(f, name)

	outChan := make(chan Event)
	backendChan := make(chan notify.EventInfo, backendBuffer)

	eventMask := subEventMask
	if !ignorePerms {
		eventMask |= permEventMask
	}

	absShouldIgnore := func(absPath string) bool {
		if !utf8.ValidString(absPath) {
			return true
		}

		rel, err := f.unrootedChecked(absPath, roots)
		if err != nil {
			return true
		}
		return ignore.Match(rel).CanSkipDir()
	}
	err = notify.WatchWithFilter(watchPath, backendChan, absShouldIgnore, eventMask)
	if err != nil {
		notify.Stop(backendChan)
		// Add Windows-specific error messages
		if build.IsWindows && isWindowsWatchingError(err) {
			l.Debugln(f.Type(), f.URI(), "Watch: Windows file watching limitation encountered. Consider excluding large directories or using manual scans.")
		}
		if reachedMaxUserWatches(err) {
			err = errors.New("failed to set up inotify handler. Please increase inotify limits, see https://docs.syncthing.net/users/faq.html#inotify-limits")
		}
		return nil, nil, err
	}

	errChan := make(chan error)
	go f.watchLoop(ctx, name, roots, backendChan, outChan, errChan, ignore)

	return outChan, errChan, nil
}

// isWindowsWatchingError checks if an error is a Windows-specific watching error
func isWindowsWatchingError(err error) bool {
	// Common Windows file watching errors
	errorString := err.Error()
	windowsErrors := []string{
		"parameter is incorrect",
		"operation was cancelled",
		"access is denied",
		"file system does not support file change notifications",
	}

	for _, winErr := range windowsErrors {
		if strings.Contains(strings.ToLower(errorString), winErr) {
			return true
		}
	}

	return false
}

func (f *BasicFilesystem) watchLoop(ctx context.Context, name string, roots []string, backendChan chan notify.EventInfo, outChan chan<- Event, errChan chan<- error, ignore Matcher) {
	// Initialize overflow tracking for adaptive buffer management
	overflowTracker := newOverflowTracker()

	// Initialize metrics tracking
	metrics := newWatchMetrics()
	metrics.logMetrics(f, name) // Start periodic logging

	for {
		// Detect channel overflow
		if len(backendChan) == backendBuffer {
		outer:
			for {
				select {
				case <-backendChan:
					metrics.recordDroppedEvent() // Record dropped events
				default:
					break outer
				}
			}
			// Record the overflow for adaptive management
			overflowTracker.recordOverflow()
			metrics.recordOverflow() // Record for metrics

			// When next scheduling a scan, do it on the entire folder as events have been lost.
			outChan <- Event{Name: name, Type: NonRemove}
			l.Debugln(f.Type(), f.URI(), "Watch: Event overflow, send \".\"")
			// Log a warning when buffer overflows to help with debugging
			l.Debugln(f.Type(), f.URI(), "Watch: Event buffer overflow detected. Consider increasing buffer size or reducing file change frequency.")

			// Check if we should increase the buffer size based on overflow patterns
			if overflowTracker.shouldIncreaseBuffer() {
				newSize := overflowTracker.increaseBuffer()
				l.Debugln(f.Type(), f.URI(), "Watch: Increasing adaptive buffer size to", newSize, "due to frequent overflows")
			}
		}

		select {
		case ev := <-backendChan:
			evPath := ev.Path()

			if !utf8.ValidString(evPath) {
				l.Debugln(f.Type(), f.URI(), "Watch: Ignoring invalid UTF-8")
				continue
			}

			relPath, err := f.unrootedChecked(evPath, roots)
			if err != nil {
				select {
				case errChan <- err:
					l.Debugln(f.Type(), f.URI(), "Watch: Sending error", err)
				case <-ctx.Done():
				}
				notify.Stop(backendChan)
				l.Debugln(f.Type(), f.URI(), "Watch: Stopped due to", err)
				return
			}

			if ignore.Match(relPath).IsIgnored() {
				l.Debugln(f.Type(), f.URI(), "Watch: Ignoring", relPath)
				continue
			}
			evType := f.eventType(ev.Event())
			select {
			case outChan <- Event{Name: relPath, Type: evType}:
				metrics.recordEvent() // Record processed event
				l.Debugln(f.Type(), f.URI(), "Watch: Sending", relPath, evType)
			case <-ctx.Done():
				notify.Stop(backendChan)
				l.Debugln(f.Type(), f.URI(), "Watch: Stopped")
				return
			}
		case <-ctx.Done():
			notify.Stop(backendChan)
			// Log final metrics when stopping
			eventsProcessed, eventsDropped, overflows, _, _ := metrics.getMetrics()
			l.Debugln(f.Type(), f.URI(), "Watch: Stopped. Final metrics - Processed:", eventsProcessed,
				"Dropped:", eventsDropped, "Overflows:", overflows)
			return
		}
	}
}

func (*BasicFilesystem) eventType(notifyType notify.Event) EventType {
	if notifyType&rmEventMask != 0 {
		return Remove
	}
	return NonRemove
}

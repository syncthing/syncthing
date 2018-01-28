// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"
	"errors"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/watchaggregator"
)

var errWatchNotStarted error = errors.New("not started")

type folder struct {
	stateTracker
	config.FolderConfiguration

	scan                folderScanner
	model               *Model
	ctx                 context.Context
	cancel              context.CancelFunc
	initialScanFinished chan struct{}
	watchCancel         context.CancelFunc
	watchChan           chan []string
	restartWatchChan    chan struct{}
	watchErr            error
	watchErrMut         sync.Mutex
}

func newFolder(model *Model, cfg config.FolderConfiguration) folder {
	ctx, cancel := context.WithCancel(context.Background())

	return folder{
		stateTracker:        newStateTracker(cfg.ID),
		FolderConfiguration: cfg,

		scan:                newFolderScanner(cfg),
		ctx:                 ctx,
		cancel:              cancel,
		model:               model,
		initialScanFinished: make(chan struct{}),
		watchCancel:         func() {},
		watchErr:            errWatchNotStarted,
		watchErrMut:         sync.NewMutex(),
	}
}

func (f *folder) BringToFront(string) {}

func (f *folder) DelayScan(next time.Duration) {
	f.scan.Delay(next)
}

func (f *folder) IgnoresUpdated() {
	if f.FSWatcherEnabled {
		f.scheduleWatchRestart()
	}
}

func (f *folder) SchedulePull() {}

func (f *folder) Jobs() ([]string, []string) {
	return nil, nil
}

func (f *folder) Scan(subdirs []string) error {
	<-f.initialScanFinished
	return f.scan.Scan(subdirs)
}

func (f *folder) Stop() {
	f.cancel()
}

// CheckHealth checks the folder for common errors, updates the folder state
// and returns the current folder error, or nil if the folder is healthy.
func (f *folder) CheckHealth() error {
	err := f.getHealthError()
	f.setError(err)
	return err
}

func (f *folder) getHealthError() error {
	// Check for folder errors, with the most serious and specific first and
	// generic ones like out of space on the home disk later.

	if err := f.CheckPath(); err != nil {
		return err
	}

	if err := f.CheckFreeSpace(); err != nil {
		return err
	}

	if err := f.model.cfg.CheckHomeFreeSpace(); err != nil {
		return err
	}

	return nil
}

func (f *folder) scanSubdirs(subDirs []string) error {
	if err := f.model.internalScanFolderSubdirs(f.ctx, f.folderID, subDirs); err != nil {
		// Potentially sets the error twice, once in the scanner just
		// by doing a check, and once here, if the error returned is
		// the same one as returned by CheckHealth, though
		// duplicate set is handled by setError.
		f.setError(err)
		return err
	}
	return nil
}

func (f *folder) scanTimerFired() {
	err := f.scanSubdirs(nil)

	select {
	case <-f.initialScanFinished:
	default:
		status := "Completed"
		if err != nil {
			status = "Failed"
		}
		l.Infoln(status, "initial scan of", f.Type.String(), "folder", f.Description())
		close(f.initialScanFinished)
	}

	f.scan.Reschedule()
}

func (f *folder) WatchError() error {
	f.watchErrMut.Lock()
	defer f.watchErrMut.Unlock()
	return f.watchErr
}

// stopWatch immediately aborts watching and may be called asynchronously
func (f *folder) stopWatch() {
	f.watchCancel()
	f.watchErrMut.Lock()
	f.watchErr = errWatchNotStarted
	f.watchErrMut.Unlock()
}

// scheduleWatchRestart makes sure watching is restarted from the main for loop
// in a folder's Serve and thus may be called asynchronously (e.g. when ignores change).
func (f *folder) scheduleWatchRestart() {
	select {
	case f.restartWatchChan <- struct{}{}:
	default:
		// We might be busy doing a pull and thus not reading from this
		// channel. The channel is 1-buffered, so one notification will be
		// queued to ensure we recheck after the pull.
	}
}

// restartWatch should only ever be called synchronously. If you want to use
// this asynchronously, you should probably use scheduleWatchRestart instead.
func (f *folder) restartWatch() {
	f.stopWatch()
	f.startWatch()
	f.Scan(nil)
}

// startWatch should only ever be called synchronously. If you want to use
// this asynchronously, you should probably use scheduleWatchRestart instead.
func (f *folder) startWatch() {
	ctx, cancel := context.WithCancel(f.ctx)
	f.model.fmut.RLock()
	ignores := f.model.folderIgnores[f.folderID]
	f.model.fmut.RUnlock()
	f.watchChan = make(chan []string)
	f.watchCancel = cancel
	go f.startWatchAsync(ctx, ignores)
}

// startWatchAsync tries to start the filesystem watching and retries every minute on failure.
// It is a convenience function that should not be used except in startWatch.
func (f *folder) startWatchAsync(ctx context.Context, ignores *ignore.Matcher) {
	timer := time.NewTimer(0)
	for {
		select {
		case <-timer.C:
			eventChan, err := f.Filesystem().Watch(".", ignores, ctx, f.IgnorePerms)
			f.watchErrMut.Lock()
			prevErr := f.watchErr
			f.watchErr = err
			f.watchErrMut.Unlock()
			if err != nil {
				if prevErr == errWatchNotStarted {
					l.Warnf("Failed to start filesystem watcher for folder %s: %v", f.Description(), err)
				} else {
					l.Debugf("Failed to start filesystem watcher for folder %s again: %v", f.Description(), err)
				}
				timer.Reset(time.Minute)
				continue
			}
			watchaggregator.Aggregate(eventChan, f.watchChan, f.FolderConfiguration, f.model.cfg, ctx)
			l.Debugln("Started filesystem watcher for folder", f.Description())
			return
		case <-ctx.Done():
			return
		}
	}
}

func (f *folder) setError(err error) {
	_, _, oldErr := f.getState()
	if (err != nil && oldErr != nil && oldErr.Error() == err.Error()) || (err == nil && oldErr == nil) {
		return
	}

	if err != nil {
		if oldErr == nil {
			l.Warnf("Error on folder %s: %v", f.Description(), err)
		} else {
			l.Infof("Error on folder %s changed: %q -> %q", f.Description(), oldErr, err)
		}
	} else {
		l.Infoln("Cleared error on folder", f.Description())
	}

	if f.FSWatcherEnabled {
		if err != nil {
			f.stopWatch()
		} else {
			f.scheduleWatchRestart()
		}
	}

	f.stateTracker.setError(err)
}

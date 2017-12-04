// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/watchaggregator"
)

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

func (f *folder) startWatch() {
	ctx, cancel := context.WithCancel(f.ctx)
	f.model.fmut.RLock()
	ignores := f.model.folderIgnores[f.folderID]
	f.model.fmut.RUnlock()
	eventChan, err := f.Filesystem().Watch(".", ignores, ctx, f.IgnorePerms)
	if err != nil {
		l.Warnf("Failed to start filesystem watcher for folder %s: %v", f.Description(), err)
	} else {
		f.watchChan = make(chan []string)
		f.watchCancel = cancel
		watchaggregator.Aggregate(eventChan, f.watchChan, f.FolderConfiguration, f.model.cfg, ctx)
		l.Infoln("Started filesystem watcher for folder", f.Description())
	}
}

func (f *folder) restartWatch() {
	f.watchCancel()
	f.startWatch()
	f.Scan(nil)
}

func (f *folder) scheduleWatchRestart() {
	select {
	case f.restartWatchChan <- struct{}{}:
	default:
		// We might be busy doing a pull and thus not reading from this
		// channel. The channel is 1-buffered, so one notification will be
		// queued to ensure we recheck after the pull.
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
			f.watchCancel()
		} else {
			f.scheduleWatchRestart()
		}
	}

	f.stateTracker.setError(err)
}

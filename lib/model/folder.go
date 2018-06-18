// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/watchaggregator"
)

var errWatchNotStarted = errors.New("not started")

type folder struct {
	stateTracker
	config.FolderConfiguration

	model   *Model
	shortID protocol.ShortID
	ctx     context.Context
	cancel  context.CancelFunc

	scanInterval        time.Duration
	scanTimer           *time.Timer
	scanNow             chan rescanRequest
	scanDelay           chan time.Duration
	initialScanFinished chan struct{}
	stopped             chan struct{}

	pullScheduled chan struct{}

	watchCancel      context.CancelFunc
	watchChan        chan []string
	restartWatchChan chan struct{}
	watchErr         error
	watchErrMut      sync.Mutex

	puller puller
}

type rescanRequest struct {
	subdirs []string
	err     chan error
}

type puller interface {
	pull() bool // true when successfull and should not be retried
}

func newFolder(model *Model, cfg config.FolderConfiguration) folder {
	ctx, cancel := context.WithCancel(context.Background())

	return folder{
		stateTracker:        newStateTracker(cfg.ID),
		FolderConfiguration: cfg,

		model:   model,
		shortID: model.shortID,
		ctx:     ctx,
		cancel:  cancel,

		scanInterval:        time.Duration(cfg.RescanIntervalS) * time.Second,
		scanTimer:           time.NewTimer(time.Millisecond), // The first scan should be done immediately.
		scanNow:             make(chan rescanRequest),
		scanDelay:           make(chan time.Duration),
		initialScanFinished: make(chan struct{}),
		stopped:             make(chan struct{}),

		pullScheduled: make(chan struct{}, 1), // This needs to be 1-buffered so that we queue a pull if we're busy when it comes.

		watchCancel:      func() {},
		restartWatchChan: make(chan struct{}, 1),
		watchErrMut:      sync.NewMutex(),
	}
}

func (f *folder) Serve() {
	l.Debugln(f, "starting")
	defer l.Debugln(f, "exiting")

	defer func() {
		f.scanTimer.Stop()
		f.setState(FolderIdle)
		close(f.stopped)
	}()

	pause := f.basePause()
	pullFailTimer := time.NewTimer(0)
	<-pullFailTimer.C

	if f.FSWatcherEnabled && f.CheckHealth() == nil {
		f.startWatch()
	}

	initialCompleted := f.initialScanFinished

	for {
		select {
		case <-f.ctx.Done():
			return

		case <-f.pullScheduled:
			pullFailTimer.Stop()
			select {
			case <-pullFailTimer.C:
			default:
			}

			if !f.puller.pull() {
				// Pulling failed, try again later.
				pullFailTimer.Reset(pause)
			}

		case <-pullFailTimer.C:
			if f.puller.pull() {
				// We're good. Don't schedule another fail pull and reset
				// the pause interval.
				pause = f.basePause()
				continue
			}

			// Pulling failed, try again later.
			l.Infof("Folder %v isn't making sync progress - retrying in %v.", f.Description(), pause)
			pullFailTimer.Reset(pause)
			// Back off from retrying to pull with an upper limit.
			if pause < 60*f.basePause() {
				pause *= 2
			}

		case <-initialCompleted:
			// Initial scan has completed, we should do a pull
			initialCompleted = nil // never hit this case again
			if !f.puller.pull() {
				// Pulling failed, try again later.
				pullFailTimer.Reset(pause)
			}

		// The reason for running the scanner from within the puller is that
		// this is the easiest way to make sure we are not doing both at the
		// same time.
		case <-f.scanTimer.C:
			l.Debugln(f, "Scanning subdirectories")
			f.scanTimerFired()

		case req := <-f.scanNow:
			req.err <- f.scanSubdirs(req.subdirs)

		case next := <-f.scanDelay:
			f.scanTimer.Reset(next)

		case fsEvents := <-f.watchChan:
			l.Debugln(f, "filesystem notification rescan")
			f.scanSubdirs(fsEvents)

		case <-f.restartWatchChan:
			f.restartWatch()
		}
	}
}

func (f *folder) BringToFront(string) {}

func (f *folder) Override(fs *db.FileSet, updateFn func([]protocol.FileInfo)) {}

func (f *folder) DelayScan(next time.Duration) {
	f.Delay(next)
}

func (f *folder) IgnoresUpdated() {
	if f.FSWatcherEnabled {
		f.scheduleWatchRestart()
	}
}

func (f *folder) SchedulePull() {
	select {
	case f.pullScheduled <- struct{}{}:
	default:
		// We might be busy doing a pull and thus not reading from this
		// channel. The channel is 1-buffered, so one notification will be
		// queued to ensure we recheck after the pull, but beyond that we must
		// make sure to not block index receiving.
	}
}

func (f *folder) Jobs() ([]string, []string) {
	return nil, nil
}

func (f *folder) Scan(subdirs []string) error {
	<-f.initialScanFinished
	req := rescanRequest{
		subdirs: subdirs,
		err:     make(chan error),
	}
	f.scanNow <- req
	return <-req.err
}

func (f *folder) Reschedule() {
	if f.scanInterval == 0 {
		return
	}
	// Sleep a random time between 3/4 and 5/4 of the configured interval.
	sleepNanos := (f.scanInterval.Nanoseconds()*3 + rand.Int63n(2*f.scanInterval.Nanoseconds())) / 4
	interval := time.Duration(sleepNanos) * time.Nanosecond
	l.Debugln(f, "next rescan in", interval)
	f.scanTimer.Reset(interval)
}

func (f *folder) Delay(next time.Duration) {
	f.scanDelay <- next
}

func (f *folder) Stop() {
	f.cancel()
	<-f.stopped
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

	f.Reschedule()
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
	prevErr := f.watchErr
	f.watchErr = errWatchNotStarted
	f.watchErrMut.Unlock()
	if prevErr != errWatchNotStarted {
		data := map[string]interface{}{
			"folder": f.ID,
			"to":     errWatchNotStarted.Error(),
		}
		if prevErr != nil {
			data["from"] = prevErr.Error()
		}
		events.Default.Log(events.FolderWatchStateChanged, data)
	}
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
	f.scanSubdirs(nil)
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
			if err != prevErr {
				data := map[string]interface{}{
					"folder": f.ID,
				}
				if prevErr != nil {
					data["from"] = prevErr.Error()
				}
				if err != nil {
					data["to"] = err.Error()
				}
				events.Default.Log(events.FolderWatchStateChanged, data)
			}
			if err != nil {
				if prevErr == errWatchNotStarted {
					l.Infof("Error while trying to start filesystem watcher for folder %s, trying again in 1min: %v", f.Description(), err)
				} else {
					l.Debugf("Repeat error while trying to start filesystem watcher for folder %s, trying again in 1min: %v", f.Description(), err)
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

func (f *folder) basePause() time.Duration {
	if f.PullerPauseS == 0 {
		return defaultPullerPause
	}
	return time.Duration(f.PullerPauseS) * time.Second
}

func (f *folder) String() string {
	return fmt.Sprintf("%s/%s@%p", f.Type, f.folderID, f)
}

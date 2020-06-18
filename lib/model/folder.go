// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"sort"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
	"github.com/syncthing/syncthing/lib/stats"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/util"
	"github.com/syncthing/syncthing/lib/watchaggregator"

	"github.com/thejerf/suture"
)

type folder struct {
	suture.Service
	stateTracker
	config.FolderConfiguration
	*stats.FolderStatisticsReference
	ioLimiter *byteSemaphore

	localFlags uint32

	model   *model
	shortID protocol.ShortID
	fset    *db.FileSet
	ignores *ignore.Matcher
	ctx     context.Context

	scanInterval        time.Duration
	scanTimer           *time.Timer
	scanDelay           chan time.Duration
	initialScanFinished chan struct{}
	scanErrors          []FileError
	scanErrorsMut       sync.Mutex

	pullScheduled chan struct{}
	pullPause     time.Duration
	pullFailTimer *time.Timer

	doInSyncChan chan syncRequest

	forcedRescanRequested chan struct{}
	forcedRescanPaths     map[string]struct{}
	forcedRescanPathsMut  sync.Mutex

	watchCancel      context.CancelFunc
	watchChan        chan []string
	restartWatchChan chan struct{}
	watchErr         error
	watchMut         sync.Mutex

	puller puller
}

type syncRequest struct {
	fn  func() error
	err chan error
}

type puller interface {
	pull() bool // true when successfull and should not be retried
}

func newFolder(model *model, fset *db.FileSet, ignores *ignore.Matcher, cfg config.FolderConfiguration, evLogger events.Logger, ioLimiter *byteSemaphore) folder {
	f := folder{
		stateTracker:              newStateTracker(cfg.ID, evLogger),
		FolderConfiguration:       cfg,
		FolderStatisticsReference: stats.NewFolderStatisticsReference(model.db, cfg.ID),
		ioLimiter:                 ioLimiter,

		model:   model,
		shortID: model.shortID,
		fset:    fset,
		ignores: ignores,

		scanInterval:        time.Duration(cfg.RescanIntervalS) * time.Second,
		scanTimer:           time.NewTimer(time.Millisecond), // The first scan should be done immediately.
		scanDelay:           make(chan time.Duration),
		initialScanFinished: make(chan struct{}),
		scanErrorsMut:       sync.NewMutex(),

		pullScheduled: make(chan struct{}, 1), // This needs to be 1-buffered so that we queue a pull if we're busy when it comes.

		doInSyncChan: make(chan syncRequest),

		forcedRescanRequested: make(chan struct{}, 1),
		forcedRescanPaths:     make(map[string]struct{}),
		forcedRescanPathsMut:  sync.NewMutex(),

		watchCancel:      func() {},
		restartWatchChan: make(chan struct{}, 1),
		watchMut:         sync.NewMutex(),
	}
	f.pullPause = f.pullBasePause()
	f.pullFailTimer = time.NewTimer(0)
	<-f.pullFailTimer.C
	return f
}

func (f *folder) serve(ctx context.Context) {
	atomic.AddInt32(&f.model.foldersRunning, 1)
	defer atomic.AddInt32(&f.model.foldersRunning, -1)

	f.ctx = ctx

	l.Debugln(f, "starting")
	defer l.Debugln(f, "exiting")

	defer func() {
		f.scanTimer.Stop()
		f.setState(FolderIdle)
	}()

	if f.FSWatcherEnabled && f.getHealthErrorAndLoadIgnores() == nil {
		f.startWatch()
	}

	initialCompleted := f.initialScanFinished

	for {
		select {
		case <-f.ctx.Done():
			return

		case <-f.pullScheduled:
			f.pull()

		case <-f.pullFailTimer.C:
			if !f.pull() && f.pullPause < 60*f.pullBasePause() {
				// Back off from retrying to pull
				f.pullPause *= 2
			}

		case <-initialCompleted:
			// Initial scan has completed, we should do a pull
			initialCompleted = nil // never hit this case again
			f.pull()

		case <-f.forcedRescanRequested:
			f.handleForcedRescans()

		case <-f.scanTimer.C:
			l.Debugln(f, "Scanning due to timer")
			f.scanTimerFired()

		case req := <-f.doInSyncChan:
			l.Debugln(f, "Running something due to request")
			req.err <- req.fn()

		case next := <-f.scanDelay:
			l.Debugln(f, "Delaying scan")
			f.scanTimer.Reset(next)

		case fsEvents := <-f.watchChan:
			l.Debugln(f, "Scan due to watcher")
			f.scanSubdirs(fsEvents)

		case <-f.restartWatchChan:
			l.Debugln(f, "Restart watcher")
			f.restartWatch()
		}
	}
}

func (f *folder) BringToFront(string) {}

func (f *folder) Override() {}

func (f *folder) Revert() {}

func (f *folder) DelayScan(next time.Duration) {
	f.Delay(next)
}

func (f *folder) ignoresUpdated() {
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

func (f *folder) Jobs(_, _ int) ([]string, []string, int) {
	return nil, nil, 0
}

func (f *folder) Scan(subdirs []string) error {
	<-f.initialScanFinished
	return f.doInSync(func() error { return f.scanSubdirs(subdirs) })
}

// doInSync allows to run functions synchronously in folder.serve from exported,
// asynchronously called methods.
func (f *folder) doInSync(fn func() error) error {
	req := syncRequest{
		fn:  fn,
		err: make(chan error, 1),
	}

	select {
	case f.doInSyncChan <- req:
		return <-req.err
	case <-f.ctx.Done():
		return f.ctx.Err()
	}
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

func (f *folder) getHealthErrorAndLoadIgnores() error {
	if err := f.getHealthErrorWithoutIgnores(); err != nil {
		return err
	}
	if err := f.ignores.Load(".stignore"); err != nil && !fs.IsNotExist(err) {
		return errors.Wrap(err, "loading ignores")
	}
	return nil
}

func (f *folder) getHealthErrorWithoutIgnores() error {
	// Check for folder errors, with the most serious and specific first and
	// generic ones like out of space on the home disk later.

	if err := f.CheckPath(); err != nil {
		return err
	}

	dbPath := locations.Get(locations.Database)
	if usage, err := fs.NewFilesystem(fs.FilesystemTypeBasic, dbPath).Usage("."); err == nil {
		if err = config.CheckFreeSpace(f.model.cfg.Options().MinHomeDiskFree, usage); err != nil {
			return errors.Wrapf(err, "insufficient space on disk for database (%v)", dbPath)
		}
	}

	return nil
}

func (f *folder) pull() (success bool) {
	f.pullFailTimer.Stop()
	select {
	case <-f.pullFailTimer.C:
	default:
	}

	select {
	case <-f.initialScanFinished:
	default:
		// Once the initial scan finished, a pull will be scheduled
		return true
	}

	defer func() {
		if success {
			// We're good, reset the pause interval.
			f.pullPause = f.pullBasePause()
		}
	}()

	// If there is nothing to do, don't even enter sync-waiting state.
	abort := true
	snap := f.fset.Snapshot()
	snap.WithNeed(protocol.LocalDeviceID, func(intf protocol.FileIntf) bool {
		abort = false
		return false
	})
	snap.Release()
	if abort {
		return true
	}

	f.setState(FolderSyncWaiting)
	defer f.setState(FolderIdle)

	if err := f.ioLimiter.takeWithContext(f.ctx, 1); err != nil {
		return true
	}
	defer f.ioLimiter.give(1)

	startTime := time.Now()

	success = f.puller.pull()

	if success {
		return true
	}

	// Pulling failed, try again later.
	delay := f.pullPause + time.Since(startTime)
	l.Infof("Folder %v isn't making sync progress - retrying in %v.", f.Description(), util.NiceDurationString(delay))
	f.pullFailTimer.Reset(delay)
	return false
}

func (f *folder) scanSubdirs(subDirs []string) error {
	oldHash := f.ignores.Hash()

	err := f.getHealthErrorAndLoadIgnores()
	f.setError(err)
	if err != nil {
		// If there is a health error we set it as the folder error. We do not
		// clear the folder error if there is no health error, as there might be
		// an *other* folder error (failed to load ignores, for example). Hence
		// we do not use the CheckHealth() convenience function here.
		return err
	}

	// Check on the way out if the ignore patterns changed as part of scanning
	// this folder. If they did we should schedule a pull of the folder so that
	// we request things we might have suddenly become unignored and so on.
	defer func() {
		if f.ignores.Hash() != oldHash {
			l.Debugln("Folder", f.Description(), "ignore patterns change detected while scanning; triggering puller")
			f.ignoresUpdated()
			f.SchedulePull()
		}
	}()

	f.setState(FolderScanWaiting)
	defer f.setState(FolderIdle)

	if err := f.ioLimiter.takeWithContext(f.ctx, 1); err != nil {
		return err
	}
	defer f.ioLimiter.give(1)

	for i := range subDirs {
		sub := osutil.NativeFilename(subDirs[i])

		if sub == "" {
			// A blank subdirs means to scan the entire folder. We can trim
			// the subDirs list and go on our way.
			subDirs = nil
			break
		}

		subDirs[i] = sub
	}

	snap := f.fset.Snapshot()
	// We release explicitly later in this function, however we might exit early
	// and it's ok to release twice.
	defer snap.Release()

	// Clean the list of subitems to ensure that we start at a known
	// directory, and don't scan subdirectories of things we've already
	// scanned.
	subDirs = unifySubs(subDirs, func(file string) bool {
		_, ok := snap.Get(protocol.LocalDeviceID, file)
		return ok
	})

	f.setState(FolderScanning)

	// If we return early e.g. due to a folder health error, the scan needs
	// to be cancelled.
	scanCtx, scanCancel := context.WithCancel(f.ctx)
	defer scanCancel()
	mtimefs := f.fset.MtimeFS()
	fchan := scanner.Walk(scanCtx, scanner.Config{
		Folder:                f.ID,
		Subs:                  subDirs,
		Matcher:               f.ignores,
		TempLifetime:          time.Duration(f.model.cfg.Options().KeepTemporariesH) * time.Hour,
		CurrentFiler:          cFiler{snap},
		Filesystem:            mtimefs,
		IgnorePerms:           f.IgnorePerms,
		AutoNormalize:         f.AutoNormalize,
		Hashers:               f.model.numHashers(f.ID),
		ShortID:               f.shortID,
		ProgressTickIntervalS: f.ScanProgressIntervalS,
		LocalFlags:            f.localFlags,
		ModTimeWindow:         f.ModTimeWindow(),
		EventLogger:           f.evLogger,
	})

	batchFn := func(fs []protocol.FileInfo) error {
		if err := f.getHealthErrorWithoutIgnores(); err != nil {
			l.Debugf("Stopping scan of folder %s due to: %s", f.Description(), err)
			return err
		}
		f.updateLocalsFromScanning(fs)
		return nil
	}
	// Resolve items which are identical with the global state.
	if f.localFlags&protocol.FlagLocalReceiveOnly != 0 {
		oldBatchFn := batchFn // can't reference batchFn directly (recursion)
		batchFn = func(fs []protocol.FileInfo) error {
			for i := range fs {
				switch gf, ok := snap.GetGlobal(fs[i].Name); {
				case !ok:
					continue
				case gf.IsEquivalentOptional(fs[i], f.ModTimeWindow(), false, false, protocol.FlagLocalReceiveOnly):
					// What we have locally is equivalent to the global file.
					fs[i].Version = fs[i].Version.Merge(gf.Version)
					fallthrough
				case fs[i].IsDeleted() && gf.IsReceiveOnlyChanged():
					// Our item is deleted and the global item is our own
					// receive only file. We can't delete file infos, so
					// we just pretend it is a normal deleted file (nobody
					// cares about that).
					fs[i].LocalFlags &^= protocol.FlagLocalReceiveOnly
				}
			}
			return oldBatchFn(fs)
		}
	}
	batch := newFileInfoBatch(batchFn)

	// Schedule a pull after scanning, but only if we actually detected any
	// changes.
	changes := 0
	defer func() {
		if changes > 0 {
			f.SchedulePull()
		}
	}()

	f.clearScanErrors(subDirs)
	alreadyUsed := make(map[string]struct{})
	for res := range fchan {
		if res.Err != nil {
			f.newScanError(res.Path, res.Err)
			continue
		}

		if err := batch.flushIfFull(); err != nil {
			return err
		}

		batch.append(res.File)
		changes++

		if f.localFlags&protocol.FlagLocalReceiveOnly == 0 {
			if nf, ok := f.findRename(snap, mtimefs, res.File, alreadyUsed); ok {
				batch.append(nf)
				changes++
			}
		}
	}

	if err := batch.flush(); err != nil {
		return err
	}

	if len(subDirs) == 0 {
		// If we have no specific subdirectories to traverse, set it to one
		// empty prefix so we traverse the entire folder contents once.
		subDirs = []string{""}
	}

	// Do a scan of the database for each prefix, to check for deleted and
	// ignored files.
	var toIgnore []db.FileInfoTruncated
	ignoredParent := ""

	snap.Release()
	snap = f.fset.Snapshot()
	defer snap.Release()

	for _, sub := range subDirs {
		var iterError error

		snap.WithPrefixedHaveTruncated(protocol.LocalDeviceID, sub, func(fi protocol.FileIntf) bool {
			select {
			case <-f.ctx.Done():
				return false
			default:
			}

			file := fi.(db.FileInfoTruncated)

			if err := batch.flushIfFull(); err != nil {
				iterError = err
				return false
			}

			if ignoredParent != "" && !fs.IsParent(file.Name, ignoredParent) {
				for _, file := range toIgnore {
					l.Debugln("marking file as ignored", file)
					nf := file.ConvertToIgnoredFileInfo(f.shortID)
					batch.append(nf)
					changes++
					if err := batch.flushIfFull(); err != nil {
						iterError = err
						return false
					}
				}
				toIgnore = toIgnore[:0]
				ignoredParent = ""
			}

			switch ignored := f.ignores.Match(file.Name).IsIgnored(); {
			case !file.IsIgnored() && ignored:
				// File was not ignored at last pass but has been ignored.
				if file.IsDirectory() {
					// Delay ignoring as a child might be unignored.
					toIgnore = append(toIgnore, file)
					if ignoredParent == "" {
						// If the parent wasn't ignored already, set
						// this path as the "highest" ignored parent
						ignoredParent = file.Name
					}
					return true
				}

				l.Debugln("marking file as ignored", file)
				nf := file.ConvertToIgnoredFileInfo(f.shortID)
				batch.append(nf)
				changes++

			case file.IsIgnored() && !ignored:
				// Successfully scanned items are already un-ignored during
				// the scan, so check whether it is deleted.
				fallthrough
			case !file.IsIgnored() && !file.IsDeleted() && !file.IsUnsupported():
				// The file is not ignored, deleted or unsupported. Lets check if
				// it's still here. Simply stat:ing it wont do as there are
				// tons of corner cases (e.g. parent dir->symlink, missing
				// permissions)
				if !osutil.IsDeleted(mtimefs, file.Name) {
					if ignoredParent != "" {
						// Don't ignore parents of this not ignored item
						toIgnore = toIgnore[:0]
						ignoredParent = ""
					}
					return true
				}
				nf := file.ConvertToDeletedFileInfo(f.shortID)
				nf.LocalFlags = f.localFlags
				if file.ShouldConflict() {
					// We do not want to override the global version with
					// the deleted file. Setting to an empty version makes
					// sure the file gets in sync on the following pull.
					nf.Version = protocol.Vector{}
				}

				batch.append(nf)
				changes++
			}

			// Check for deleted, locally changed items that noone else has.
			if f.localFlags&protocol.FlagLocalReceiveOnly == 0 {
				return true
			}
			if !fi.IsDeleted() || !fi.IsReceiveOnlyChanged() || len(snap.Availability(fi.FileName())) > 0 {
				return true
			}
			nf := fi.(db.FileInfoTruncated).ConvertDeletedToFileInfo()
			nf.LocalFlags = 0
			nf.Version = protocol.Vector{}
			batch.append(nf)
			changes++

			return true
		})

		select {
		case <-f.ctx.Done():
			return f.ctx.Err()
		default:
		}

		if iterError == nil && len(toIgnore) > 0 {
			for _, file := range toIgnore {
				l.Debugln("marking file as ignored", f)
				nf := file.ConvertToIgnoredFileInfo(f.shortID)
				batch.append(nf)
				changes++
				if iterError = batch.flushIfFull(); iterError != nil {
					break
				}
			}
			toIgnore = toIgnore[:0]
		}

		if iterError != nil {
			return iterError
		}
	}

	if err := batch.flush(); err != nil {
		return err
	}

	f.ScanCompleted()
	return nil
}

func (f *folder) findRename(snap *db.Snapshot, mtimefs fs.Filesystem, file protocol.FileInfo, alreadyUsed map[string]struct{}) (protocol.FileInfo, bool) {
	if len(file.Blocks) == 0 || file.Size == 0 {
		return protocol.FileInfo{}, false
	}

	found := false
	nf := protocol.FileInfo{}

	snap.WithBlocksHash(file.BlocksHash, func(ifi protocol.FileIntf) bool {
		fi := ifi.(protocol.FileInfo)

		select {
		case <-f.ctx.Done():
			return false
		default:
		}

		if _, ok := alreadyUsed[fi.Name]; ok {
			return true
		}

		if fi.ShouldConflict() {
			return true
		}

		if f.ignores.Match(fi.Name).IsIgnored() {
			return true
		}

		// Only check the size.
		// No point checking block equality, as that uses BlocksHash comparison if that is set (which it will be).
		// No point checking BlocksHash comparison as WithBlocksHash already does that.
		if file.Size != fi.Size {
			return true
		}

		if !osutil.IsDeleted(mtimefs, fi.Name) {
			return true
		}

		alreadyUsed[fi.Name] = struct{}{}

		nf = fi
		nf.SetDeleted(f.shortID)
		nf.LocalFlags = f.localFlags
		found = true
		return false
	})

	return nf, found
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
	f.watchMut.Lock()
	defer f.watchMut.Unlock()
	return f.watchErr
}

// stopWatch immediately aborts watching and may be called asynchronously
func (f *folder) stopWatch() {
	f.watchMut.Lock()
	f.watchCancel()
	f.watchMut.Unlock()
	f.setWatchError(nil, 0)
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
	f.watchMut.Lock()
	f.watchChan = make(chan []string)
	f.watchCancel = cancel
	f.watchMut.Unlock()
	go f.monitorWatch(ctx)
}

// monitorWatch starts the filesystem watching and retries every minute on failure.
// It should not be used except in startWatch.
func (f *folder) monitorWatch(ctx context.Context) {
	failTimer := time.NewTimer(0)
	aggrCtx, aggrCancel := context.WithCancel(ctx)
	var err error
	var eventChan <-chan fs.Event
	var errChan <-chan error
	warnedOutside := false
	var lastWatch time.Time
	pause := time.Minute
	for {
		select {
		case <-failTimer.C:
			eventChan, errChan, err = f.Filesystem().Watch(".", f.ignores, ctx, f.IgnorePerms)
			// We do this once per minute initially increased to
			// max one hour in case of repeat failures.
			f.scanOnWatchErr()
			f.setWatchError(err, pause)
			if err != nil {
				failTimer.Reset(pause)
				if pause < 60*time.Minute {
					pause *= 2
				}
				continue
			}
			lastWatch = time.Now()
			watchaggregator.Aggregate(aggrCtx, eventChan, f.watchChan, f.FolderConfiguration, f.model.cfg, f.evLogger)
			l.Debugln("Started filesystem watcher for folder", f.Description())
		case err = <-errChan:
			var next time.Duration
			if dur := time.Since(lastWatch); dur > pause {
				pause = time.Minute
				next = 0
			} else {
				next = pause - dur
				if pause < 60*time.Minute {
					pause *= 2
				}
			}
			failTimer.Reset(next)
			f.setWatchError(err, next)
			// This error was previously a panic and should never occur, so generate
			// a warning, but don't do it repetitively.
			if !warnedOutside {
				if _, ok := err.(*fs.ErrWatchEventOutsideRoot); ok {
					l.Warnln(err)
					warnedOutside = true
				}
			}
			aggrCancel()
			errChan = nil
			aggrCtx, aggrCancel = context.WithCancel(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// setWatchError sets the current error state of the watch and should be called
// regardless of whether err is nil or not.
func (f *folder) setWatchError(err error, nextTryIn time.Duration) {
	f.watchMut.Lock()
	prevErr := f.watchErr
	f.watchErr = err
	f.watchMut.Unlock()
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
		f.evLogger.Log(events.FolderWatchStateChanged, data)
	}
	if err == nil {
		return
	}
	msg := fmt.Sprintf("Error while trying to start filesystem watcher for folder %s, trying again in %v: %v", f.Description(), nextTryIn, err)
	if prevErr != err {
		l.Infof(msg)
		return
	}
	l.Debugf(msg)
}

// scanOnWatchErr schedules a full scan immediately if an error occurred while watching.
func (f *folder) scanOnWatchErr() {
	f.watchMut.Lock()
	err := f.watchErr
	f.watchMut.Unlock()
	if err != nil {
		f.Delay(0)
	}
}

func (f *folder) setError(err error) {
	select {
	case <-f.ctx.Done():
		return
	default:
	}

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

func (f *folder) pullBasePause() time.Duration {
	if f.PullerPauseS == 0 {
		return defaultPullerPause
	}
	return time.Duration(f.PullerPauseS) * time.Second
}

func (f *folder) String() string {
	return fmt.Sprintf("%s/%s@%p", f.Type, f.folderID, f)
}

func (f *folder) newScanError(path string, err error) {
	f.scanErrorsMut.Lock()
	l.Infof("Scanner (folder %s, item %q): %v", f.Description(), path, err)
	f.scanErrors = append(f.scanErrors, FileError{
		Err:  err.Error(),
		Path: path,
	})
	f.scanErrorsMut.Unlock()
}

func (f *folder) clearScanErrors(subDirs []string) {
	f.scanErrorsMut.Lock()
	defer f.scanErrorsMut.Unlock()
	if len(subDirs) == 0 {
		f.scanErrors = nil
		return
	}
	filtered := f.scanErrors[:0]
outer:
	for _, fe := range f.scanErrors {
		for _, sub := range subDirs {
			if fe.Path == sub || fs.IsParent(fe.Path, sub) {
				continue outer
			}
		}
		filtered = append(filtered, fe)
	}
	f.scanErrors = filtered
}

func (f *folder) Errors() []FileError {
	f.scanErrorsMut.Lock()
	defer f.scanErrorsMut.Unlock()
	return append([]FileError{}, f.scanErrors...)
}

// ScheduleForceRescan marks the file such that it gets rehashed on next scan, and schedules a scan.
func (f *folder) ScheduleForceRescan(path string) {
	f.forcedRescanPathsMut.Lock()
	f.forcedRescanPaths[path] = struct{}{}
	f.forcedRescanPathsMut.Unlock()

	select {
	case f.forcedRescanRequested <- struct{}{}:
	default:
	}
}

func (f *folder) updateLocalsFromScanning(fs []protocol.FileInfo) {
	f.updateLocals(fs)

	f.emitDiskChangeEvents(fs, events.LocalChangeDetected)
}

func (f *folder) updateLocalsFromPulling(fs []protocol.FileInfo) {
	f.updateLocals(fs)

	f.emitDiskChangeEvents(fs, events.RemoteChangeDetected)
}

func (f *folder) updateLocals(fs []protocol.FileInfo) {
	f.fset.Update(protocol.LocalDeviceID, fs)

	filenames := make([]string, len(fs))
	for i, file := range fs {
		filenames[i] = file.Name
	}

	f.evLogger.Log(events.LocalIndexUpdated, map[string]interface{}{
		"folder":    f.ID,
		"items":     len(fs),
		"filenames": filenames,
		"version":   f.fset.Sequence(protocol.LocalDeviceID),
	})
}

func (f *folder) emitDiskChangeEvents(fs []protocol.FileInfo, typeOfEvent events.EventType) {
	for _, file := range fs {
		if file.IsInvalid() {
			continue
		}

		objType := "file"
		action := "modified"

		if file.IsDeleted() {
			action = "deleted"
		}

		if file.IsSymlink() {
			objType = "symlink"
		} else if file.IsDirectory() {
			objType = "dir"
		}

		// Two different events can be fired here based on what EventType is passed into function
		f.evLogger.Log(typeOfEvent, map[string]string{
			"folder":     f.ID,
			"folderID":   f.ID, // incorrect, deprecated, kept for historical compliance
			"label":      f.Label,
			"action":     action,
			"type":       objType,
			"path":       filepath.FromSlash(file.Name),
			"modifiedBy": file.ModifiedBy.String(),
		})
	}
}

func (f *folder) handleForcedRescans() {
	f.forcedRescanPathsMut.Lock()
	paths := make([]string, 0, len(f.forcedRescanPaths))
	for path := range f.forcedRescanPaths {
		paths = append(paths, path)
	}
	f.forcedRescanPaths = make(map[string]struct{})
	f.forcedRescanPathsMut.Unlock()

	batch := newFileInfoBatch(func(fs []protocol.FileInfo) error {
		f.fset.Update(protocol.LocalDeviceID, fs)
		return nil
	})

	snap := f.fset.Snapshot()

	for _, path := range paths {
		_ = batch.flushIfFull()

		fi, ok := snap.Get(protocol.LocalDeviceID, path)
		if !ok {
			continue
		}
		fi.SetMustRescan(f.shortID)
		batch.append(fi)
	}

	snap.Release()

	_ = batch.flush()

	_ = f.scanSubdirs(paths)
}

// The exists function is expected to return true for all known paths
// (excluding "" and ".")
func unifySubs(dirs []string, exists func(dir string) bool) []string {
	if len(dirs) == 0 {
		return nil
	}
	sort.Strings(dirs)
	if dirs[0] == "" || dirs[0] == "." || dirs[0] == string(fs.PathSeparator) {
		return nil
	}
	prev := "./" // Anything that can't be parent of a clean path
	for i := 0; i < len(dirs); {
		dir, err := fs.Canonicalize(dirs[i])
		if err != nil {
			l.Debugf("Skipping %v for scan: %s", dirs[i], err)
			dirs = append(dirs[:i], dirs[i+1:]...)
			continue
		}
		if dir == prev || fs.IsParent(dir, prev) {
			dirs = append(dirs[:i], dirs[i+1:]...)
			continue
		}
		parent := filepath.Dir(dir)
		for parent != "." && parent != string(fs.PathSeparator) && !exists(parent) {
			dir = parent
			parent = filepath.Dir(dir)
		}
		dirs[i] = dir
		prev = dir
		i++
	}
	return dirs
}

type cFiler struct {
	*db.Snapshot
}

// Implements scanner.CurrentFiler
func (cf cFiler) CurrentFile(file string) (protocol.FileInfo, bool) {
	return cf.Get(protocol.LocalDeviceID, file)
}

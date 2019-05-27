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
	"path/filepath"
	"sort"
	"sync/atomic"
	"time"

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
	"github.com/syncthing/syncthing/lib/watchaggregator"
)

// scanLimiter limits the number of concurrent scans. A limit of zero means no limit.
var scanLimiter = newByteSemaphore(0)

var errWatchNotStarted = errors.New("not started")

type folder struct {
	stateTracker
	config.FolderConfiguration
	*stats.FolderStatisticsReference

	localFlags uint32

	model   *model
	shortID protocol.ShortID
	fset    *db.FileSet
	ignores *ignore.Matcher
	ctx     context.Context
	cancel  context.CancelFunc

	scanInterval        time.Duration
	scanTimer           *time.Timer
	scanNow             chan rescanRequest
	scanDelay           chan time.Duration
	initialScanFinished chan struct{}
	stopped             chan struct{}
	scanErrors          []FileError
	scanErrorsMut       sync.Mutex

	pullScheduled chan struct{}

	watchCancel      context.CancelFunc
	watchChan        chan []string
	restartWatchChan chan struct{}
	watchErr         error
	watchMut         sync.Mutex

	puller puller
}

type rescanRequest struct {
	subdirs []string
	err     chan error
}

type puller interface {
	pull() bool // true when successfull and should not be retried
}

func newFolder(model *model, fset *db.FileSet, ignores *ignore.Matcher, cfg config.FolderConfiguration) folder {
	ctx, cancel := context.WithCancel(context.Background())

	return folder{
		stateTracker:              newStateTracker(cfg.ID),
		FolderConfiguration:       cfg,
		FolderStatisticsReference: stats.NewFolderStatisticsReference(model.db, cfg.ID),

		model:   model,
		shortID: model.shortID,
		fset:    fset,
		ignores: ignores,
		ctx:     ctx,
		cancel:  cancel,

		scanInterval:        time.Duration(cfg.RescanIntervalS) * time.Second,
		scanTimer:           time.NewTimer(time.Millisecond), // The first scan should be done immediately.
		scanNow:             make(chan rescanRequest),
		scanDelay:           make(chan time.Duration),
		initialScanFinished: make(chan struct{}),
		stopped:             make(chan struct{}),
		scanErrorsMut:       sync.NewMutex(),

		pullScheduled: make(chan struct{}, 1), // This needs to be 1-buffered so that we queue a pull if we're busy when it comes.

		watchCancel:      func() {},
		restartWatchChan: make(chan struct{}, 1),
		watchMut:         sync.NewMutex(),
	}
}

func (f *folder) Serve() {
	atomic.AddInt32(&f.model.foldersRunning, 1)
	defer atomic.AddInt32(&f.model.foldersRunning, -1)

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

	pull := func() {
		startTime := time.Now()
		if f.puller.pull() {
			// We're good. Don't schedule another pull and reset
			// the pause interval.
			pause = f.basePause()
			return
		}
		// Pulling failed, try again later.
		delay := pause + time.Since(startTime)
		l.Infof("Folder %v isn't making sync progress - retrying in %v.", f.Description(), delay)
		pullFailTimer.Reset(delay)
		if pause < 60*f.basePause() {
			pause *= 2
		}
	}

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
			pull()

		case <-pullFailTimer.C:
			pull()

		case <-initialCompleted:
			// Initial scan has completed, we should do a pull
			initialCompleted = nil // never hit this case again
			if !f.puller.pull() {
				// Pulling failed, try again later.
				pullFailTimer.Reset(pause)
			}

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

func (f *folder) Jobs() ([]string, []string) {
	return nil, nil
}

func (f *folder) Scan(subdirs []string) error {
	<-f.initialScanFinished
	req := rescanRequest{
		subdirs: subdirs,
		err:     make(chan error),
	}

	select {
	case f.scanNow <- req:
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

	dbPath := locations.Get(locations.Database)
	if usage, err := fs.NewFilesystem(fs.FilesystemTypeBasic, dbPath).Usage("."); err == nil {
		if err = config.CheckFreeSpace(f.model.cfg.Options().MinHomeDiskFree, usage); err != nil {
			return fmt.Errorf("insufficient space on disk for database (%v): %v", dbPath, err)
		}
	}

	return nil
}

func (f *folder) scanSubdirs(subDirs []string) error {
	if err := f.CheckHealth(); err != nil {
		return err
	}

	mtimefs := f.fset.MtimeFS()

	f.setState(FolderScanWaiting)
	scanLimiter.take(1)
	defer scanLimiter.give(1)

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

	// Check if the ignore patterns changed as part of scanning this folder.
	// If they did we should schedule a pull of the folder so that we
	// request things we might have suddenly become unignored and so on.
	oldHash := f.ignores.Hash()
	defer func() {
		if f.ignores.Hash() != oldHash {
			l.Debugln("Folder", f.Description(), "ignore patterns change detected while scanning; triggering puller")
			f.ignoresUpdated()
			f.SchedulePull()
		}
	}()

	if err := f.ignores.Load(".stignore"); err != nil && !fs.IsNotExist(err) {
		err = fmt.Errorf("loading ignores: %v", err)
		f.setError(err)
		return err
	}

	// Clean the list of subitems to ensure that we start at a known
	// directory, and don't scan subdirectories of things we've already
	// scanned.
	subDirs = unifySubs(subDirs, func(file string) bool {
		_, ok := f.fset.Get(protocol.LocalDeviceID, file)
		return ok
	})

	f.setState(FolderScanning)

	fchan := scanner.Walk(f.ctx, scanner.Config{
		Folder:                f.ID,
		Subs:                  subDirs,
		Matcher:               f.ignores,
		TempLifetime:          time.Duration(f.model.cfg.Options().KeepTemporariesH) * time.Hour,
		CurrentFiler:          cFiler{f.fset},
		Filesystem:            mtimefs,
		IgnorePerms:           f.IgnorePerms,
		AutoNormalize:         f.AutoNormalize,
		Hashers:               f.model.numHashers(f.ID),
		ShortID:               f.shortID,
		ProgressTickIntervalS: f.ScanProgressIntervalS,
		UseLargeBlocks:        f.UseLargeBlocks,
		LocalFlags:            f.localFlags,
	})

	batchFn := func(fs []protocol.FileInfo) error {
		if err := f.CheckHealth(); err != nil {
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
				switch gf, ok := f.fset.GetGlobal(fs[i].Name); {
				case !ok:
					continue
				case gf.IsEquivalentOptional(fs[i], false, false, protocol.FlagLocalReceiveOnly):
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
	for _, sub := range subDirs {
		var iterError error

		f.fset.WithPrefixedHaveTruncated(protocol.LocalDeviceID, sub, func(fi db.FileIntf) bool {
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

				l.Debugln("marking file as ignored", f)
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
				nf := protocol.FileInfo{
					Name:       file.Name,
					Type:       file.Type,
					Size:       0,
					ModifiedS:  file.ModifiedS,
					ModifiedNs: file.ModifiedNs,
					ModifiedBy: f.shortID,
					Deleted:    true,
					Version:    file.Version.Update(f.shortID),
					LocalFlags: f.localFlags,
				}
				// We do not want to override the global version
				// with the deleted file. Keeping only our local
				// counter makes sure we are in conflict with any
				// other existing versions, which will be resolved
				// by the normal pulling mechanisms.
				if file.ShouldConflict() {
					nf.Version = nf.Version.DropOthers(f.shortID)
				}

				batch.append(nf)
				changes++
			}
			return true
		})

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
	f.setState(FolderIdle)
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
	f.watchMut.Lock()
	defer f.watchMut.Unlock()
	return f.watchErr
}

// stopWatch immediately aborts watching and may be called asynchronously
func (f *folder) stopWatch() {
	f.watchMut.Lock()
	f.watchCancel()
	prevErr := f.watchErr
	f.watchErr = errWatchNotStarted
	f.watchMut.Unlock()
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
	for {
		select {
		case <-failTimer.C:
			eventChan, errChan, err = f.Filesystem().Watch(".", f.ignores, ctx, f.IgnorePerms)
			// We do this at most once per minute which is the
			// default rescan time without watcher.
			f.scanOnWatchErr()
			f.setWatchError(err)
			if err != nil {
				failTimer.Reset(time.Minute)
				continue
			}
			watchaggregator.Aggregate(eventChan, f.watchChan, f.FolderConfiguration, f.model.cfg, aggrCtx)
			l.Debugln("Started filesystem watcher for folder", f.Description())
		case err = <-errChan:
			f.setWatchError(err)
			// This error was previously a panic and should never occur, so generate
			// a warning, but don't do it repetitively.
			if !warnedOutside {
				if _, ok := err.(*fs.ErrWatchEventOutsideRoot); ok {
					l.Warnln(err)
					warnedOutside = true
					return
				}
			}
			aggrCancel()
			errChan = nil
			aggrCtx, aggrCancel = context.WithCancel(ctx)
			failTimer.Reset(time.Minute)
		case <-ctx.Done():
			return
		}
	}
}

// setWatchError sets the current error state of the watch and should be called
// regardless of whether err is nil or not.
func (f *folder) setWatchError(err error) {
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
		events.Default.Log(events.FolderWatchStateChanged, data)
	}
	if err == nil {
		return
	}
	if prevErr == errWatchNotStarted {
		l.Infof("Error while trying to start filesystem watcher for folder %s, trying again in 1min: %v", f.Description(), err)
		return
	}
	l.Debugf("Repeat error while trying to start filesystem watcher for folder %s, trying again in 1min: %v", f.Description(), err)
}

// scanOnWatchErr schedules a full scan immediately if an error occurred while watching.
func (f *folder) scanOnWatchErr() {
	f.watchMut.Lock()
	if f.watchErr != nil && f.watchErr != errWatchNotStarted {
		f.Delay(0)
	}
	f.watchMut.Unlock()
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

func (f *folder) basePause() time.Duration {
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

// ForceRescan marks the file such that it gets rehashed on next scan and then
// immediately executes that scan.
func (f *folder) ForceRescan(file protocol.FileInfo) error {
	file.SetMustRescan(f.shortID)
	f.fset.Update(protocol.LocalDeviceID, []protocol.FileInfo{file})

	return f.Scan([]string{file.Name})
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

	events.Default.Log(events.LocalIndexUpdated, map[string]interface{}{
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

		switch {
		case file.IsDeleted():
			action = "deleted"

		// If our local vector is version 1 AND it is the only version
		// vector so far seen for this file then it is a new file.  Else if
		// it is > 1 it's not new, and if it is 1 but another shortId
		// version vector exists then it is new for us but created elsewhere
		// so the file is still not new but modified by us. Only if it is
		// truly new do we change this to 'added', else we leave it as
		// 'modified'.
		case len(file.Version.Counters) == 1 && file.Version.Counters[0].Value == 1:
			action = "added"
		}

		if file.IsSymlink() {
			objType = "symlink"
		} else if file.IsDirectory() {
			objType = "dir"
		}

		// Two different events can be fired here based on what EventType is passed into function
		events.Default.Log(typeOfEvent, map[string]string{
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
	*db.FileSet
}

// Implements scanner.CurrentFiler
func (cf cFiler) CurrentFile(file string) (protocol.FileInfo, bool) {
	return cf.Get(protocol.LocalDeviceID, file)
}

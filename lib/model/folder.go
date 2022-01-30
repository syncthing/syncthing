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
	"github.com/syncthing/syncthing/lib/svcutil"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/util"
	"github.com/syncthing/syncthing/lib/versioner"
	"github.com/syncthing/syncthing/lib/watchaggregator"
)

type folder struct {
	stateTracker
	config.FolderConfiguration
	*stats.FolderStatisticsReference
	ioLimiter *util.Semaphore

	localFlags uint32

	model         *model
	shortID       protocol.ShortID
	fset          *db.FileSet
	ignores       *ignore.Matcher
	mtimefs       fs.Filesystem
	modTimeWindow time.Duration
	ctx           context.Context // used internally, only accessible on serve lifetime
	done          chan struct{}   // used externally, accessible regardless of serve

	scanInterval           time.Duration
	scanTimer              *time.Timer
	scanDelay              chan time.Duration
	initialScanFinished    chan struct{}
	scanScheduled          chan struct{}
	versionCleanupInterval time.Duration
	versionCleanupTimer    *time.Timer

	pullScheduled chan struct{}
	pullPause     time.Duration
	pullFailTimer *time.Timer

	scanErrors []FileError
	pullErrors []FileError
	errorsMut  sync.Mutex

	doInSyncChan chan syncRequest

	forcedRescanRequested chan struct{}
	forcedRescanPaths     map[string]struct{}
	forcedRescanPathsMut  sync.Mutex

	watchCancel      context.CancelFunc
	watchChan        chan []string
	restartWatchChan chan struct{}
	watchErr         error
	watchMut         sync.Mutex

	puller    puller
	versioner versioner.Versioner
}

type syncRequest struct {
	fn  func() error
	err chan error
}

type puller interface {
	pull() (bool, error) // true when successful and should not be retried
}

func newFolder(model *model, fset *db.FileSet, ignores *ignore.Matcher, cfg config.FolderConfiguration, evLogger events.Logger, ioLimiter *util.Semaphore, ver versioner.Versioner) folder {
	f := folder{
		stateTracker:              newStateTracker(cfg.ID, evLogger),
		FolderConfiguration:       cfg,
		FolderStatisticsReference: stats.NewFolderStatisticsReference(model.db, cfg.ID),
		ioLimiter:                 ioLimiter,

		model:         model,
		shortID:       model.shortID,
		fset:          fset,
		ignores:       ignores,
		mtimefs:       fset.MtimeFS(cfg.Filesystem()),
		modTimeWindow: cfg.ModTimeWindow(),
		done:          make(chan struct{}),

		scanInterval:           time.Duration(cfg.RescanIntervalS) * time.Second,
		scanTimer:              time.NewTimer(0), // The first scan should be done immediately.
		scanDelay:              make(chan time.Duration),
		initialScanFinished:    make(chan struct{}),
		scanScheduled:          make(chan struct{}, 1),
		versionCleanupInterval: time.Duration(cfg.Versioning.CleanupIntervalS) * time.Second,
		versionCleanupTimer:    time.NewTimer(time.Duration(cfg.Versioning.CleanupIntervalS) * time.Second),

		pullScheduled: make(chan struct{}, 1), // This needs to be 1-buffered so that we queue a pull if we're busy when it comes.

		errorsMut: sync.NewMutex(),

		doInSyncChan: make(chan syncRequest),

		forcedRescanRequested: make(chan struct{}, 1),
		forcedRescanPaths:     make(map[string]struct{}),
		forcedRescanPathsMut:  sync.NewMutex(),

		watchCancel:      func() {},
		restartWatchChan: make(chan struct{}, 1),
		watchMut:         sync.NewMutex(),

		versioner: ver,
	}
	f.pullPause = f.pullBasePause()
	f.pullFailTimer = time.NewTimer(0)
	<-f.pullFailTimer.C
	return f
}

func (f *folder) Serve(ctx context.Context) error {
	atomic.AddInt32(&f.model.foldersRunning, 1)
	defer atomic.AddInt32(&f.model.foldersRunning, -1)

	f.ctx = ctx

	l.Debugln(f, "starting")
	defer l.Debugln(f, "exiting")

	defer func() {
		f.scanTimer.Stop()
		f.versionCleanupTimer.Stop()
		f.setState(FolderIdle)
	}()

	if f.FSWatcherEnabled && f.getHealthErrorAndLoadIgnores() == nil {
		f.startWatch()
	}

	// If we're configured to not do version cleanup, or we don't have a
	// versioner, cancel and drain that timer now.
	if f.versionCleanupInterval == 0 || f.versioner == nil {
		if !f.versionCleanupTimer.Stop() {
			<-f.versionCleanupTimer.C
		}
	}

	initialCompleted := f.initialScanFinished

	for {
		var err error

		select {
		case <-f.ctx.Done():
			close(f.done)
			return nil

		case <-f.pullScheduled:
			_, err = f.pull()

		case <-f.pullFailTimer.C:
			var success bool
			success, err = f.pull()
			if (err != nil || !success) && f.pullPause < 60*f.pullBasePause() {
				// Back off from retrying to pull
				f.pullPause *= 2
			}

		case <-initialCompleted:
			// Initial scan has completed, we should do a pull
			initialCompleted = nil // never hit this case again
			_, err = f.pull()

		case <-f.forcedRescanRequested:
			err = f.handleForcedRescans()

		case <-f.scanTimer.C:
			l.Debugln(f, "Scanning due to timer")
			err = f.scanTimerFired()

		case req := <-f.doInSyncChan:
			l.Debugln(f, "Running something due to request")
			err = req.fn()
			req.err <- err

		case next := <-f.scanDelay:
			l.Debugln(f, "Delaying scan")
			f.scanTimer.Reset(next)

		case <-f.scanScheduled:
			l.Debugln(f, "Scan was scheduled")
			f.scanTimer.Reset(0)

		case fsEvents := <-f.watchChan:
			l.Debugln(f, "Scan due to watcher")
			err = f.scanSubdirs(fsEvents)

		case <-f.restartWatchChan:
			l.Debugln(f, "Restart watcher")
			err = f.restartWatch()

		case <-f.versionCleanupTimer.C:
			l.Debugln(f, "Doing version cleanup")
			f.versionCleanupTimerFired()
		}

		if err != nil {
			if svcutil.IsFatal(err) {
				return err
			}
			f.setError(err)
		}
	}
}

func (f *folder) BringToFront(string) {}

func (f *folder) Override() {}

func (f *folder) Revert() {}

func (f *folder) DelayScan(next time.Duration) {
	select {
	case f.scanDelay <- next:
	case <-f.done:
	}
}

func (f *folder) ScheduleScan() {
	// 1-buffered chan
	select {
	case f.scanScheduled <- struct{}{}:
	default:
	}
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
	case <-f.done:
		return context.Canceled
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

func (f *folder) getHealthErrorAndLoadIgnores() error {
	if err := f.getHealthErrorWithoutIgnores(); err != nil {
		return err
	}
	if f.Type != config.FolderTypeReceiveEncrypted {
		if err := f.ignores.Load(".stignore"); err != nil && !fs.IsNotExist(err) {
			return errors.Wrap(err, "loading ignores")
		}
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

func (f *folder) pull() (success bool, err error) {
	f.pullFailTimer.Stop()
	select {
	case <-f.pullFailTimer.C:
	default:
	}

	select {
	case <-f.initialScanFinished:
	default:
		// Once the initial scan finished, a pull will be scheduled
		return true, nil
	}

	defer func() {
		if success {
			// We're good, reset the pause interval.
			f.pullPause = f.pullBasePause()
		}
	}()

	// If there is nothing to do, don't even enter sync-waiting state.
	abort := true
	snap, err := f.dbSnapshot()
	if err != nil {
		return false, err
	}
	snap.WithNeed(protocol.LocalDeviceID, func(intf protocol.FileIntf) bool {
		abort = false
		return false
	})
	snap.Release()
	if abort {
		// Clears pull failures on items that were needed before, but aren't anymore.
		f.errorsMut.Lock()
		f.pullErrors = nil
		f.errorsMut.Unlock()
		return true, nil
	}

	// Abort early (before acquiring a token) if there's a folder error
	err = f.getHealthErrorWithoutIgnores()
	if err != nil {
		l.Debugln("Skipping pull of", f.Description(), "due to folder error:", err)
		return false, err
	}
	f.setError(nil)

	// Send only folder doesn't do any io, it only checks for out-of-sync
	// items that differ in metadata and updates those.
	if f.Type != config.FolderTypeSendOnly {
		f.setState(FolderSyncWaiting)

		if err := f.ioLimiter.TakeWithContext(f.ctx, 1); err != nil {
			return true, err
		}
		defer f.ioLimiter.Give(1)
	}

	startTime := time.Now()

	// Check if the ignore patterns changed.
	oldHash := f.ignores.Hash()
	defer func() {
		if f.ignores.Hash() != oldHash {
			f.ignoresUpdated()
		}
	}()
	err = f.getHealthErrorAndLoadIgnores()
	if err != nil {
		l.Debugln("Skipping pull of", f.Description(), "due to folder error:", err)
		return false, err
	}

	success, err = f.puller.pull()

	if success && err == nil {
		return true, nil
	}

	// Pulling failed, try again later.
	delay := f.pullPause + time.Since(startTime)
	l.Infof("Folder %v isn't making sync progress - retrying in %v.", f.Description(), util.NiceDurationString(delay))
	f.pullFailTimer.Reset(delay)

	return false, err
}

func (f *folder) scanSubdirs(subDirs []string) error {
	l.Debugf("%v scanning", f)

	oldHash := f.ignores.Hash()

	err := f.getHealthErrorAndLoadIgnores()
	if err != nil {
		// If there is a health error we set it as the folder error. We do not
		// clear the folder error if there is no health error, as there might be
		// an *other* folder error (failed to load ignores, for example). Hence
		// we do not use the CheckHealth() convenience function here.
		return err
	}
	f.setError(nil)

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

	if err := f.ioLimiter.TakeWithContext(f.ctx, 1); err != nil {
		return err
	}
	defer f.ioLimiter.Give(1)

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

	// Clean the list of subitems to ensure that we start at a known
	// directory, and don't scan subdirectories of things we've already
	// scanned.
	snap, err := f.dbSnapshot()
	if err != nil {
		return err
	}
	subDirs = unifySubs(subDirs, func(file string) bool {
		_, ok := snap.Get(protocol.LocalDeviceID, file)
		return ok
	})
	snap.Release()

	f.setState(FolderScanning)
	f.clearScanErrors(subDirs)

	batch := f.newScanBatch()

	// Schedule a pull after scanning, but only if we actually detected any
	// changes.
	changes := 0
	defer func() {
		l.Debugf("%v finished scanning, detected %v changes", f, changes)
		if changes > 0 {
			f.SchedulePull()
		}
	}()

	changesHere, err := f.scanSubdirsChangedAndNew(subDirs, batch)
	changes += changesHere
	if err != nil {
		return err
	}

	if err := batch.Flush(); err != nil {
		return err
	}

	if len(subDirs) == 0 {
		// If we have no specific subdirectories to traverse, set it to one
		// empty prefix so we traverse the entire folder contents once.
		subDirs = []string{""}
	}

	// Do a scan of the database for each prefix, to check for deleted and
	// ignored files.

	changesHere, err = f.scanSubdirsDeletedAndIgnored(subDirs, batch)
	changes += changesHere
	if err != nil {
		return err
	}

	if err := batch.Flush(); err != nil {
		return err
	}

	f.ScanCompleted()
	return nil
}

const maxToRemove = 1000

type scanBatch struct {
	f           *folder
	updateBatch *db.FileInfoBatch
	toRemove    []string
}

func (f *folder) newScanBatch() *scanBatch {
	b := &scanBatch{
		f:        f,
		toRemove: make([]string, 0, maxToRemove),
	}
	b.updateBatch = db.NewFileInfoBatch(func(fs []protocol.FileInfo) error {
		if err := b.f.getHealthErrorWithoutIgnores(); err != nil {
			l.Debugf("Stopping scan of folder %s due to: %s", b.f.Description(), err)
			return err
		}
		b.f.updateLocalsFromScanning(fs)
		return nil
	})
	return b
}

func (b *scanBatch) Remove(item string) {
	b.toRemove = append(b.toRemove, item)
}

func (b *scanBatch) flushToRemove() {
	if len(b.toRemove) > 0 {
		b.f.fset.RemoveLocalItems(b.toRemove)
		b.toRemove = b.toRemove[:0]
	}
}

func (b *scanBatch) Flush() error {
	b.flushToRemove()
	return b.updateBatch.Flush()
}

func (b *scanBatch) FlushIfFull() error {
	if len(b.toRemove) >= maxToRemove {
		b.flushToRemove()
	}
	return b.updateBatch.FlushIfFull()
}

// Update adds the fileinfo to the batch for updating, and does a few checks.
// It returns false if the checks result in the file not going to be updated or removed.
func (b *scanBatch) Update(fi protocol.FileInfo, snap *db.Snapshot) bool {
	// Check for a "virtual" parent directory of encrypted files. We don't track
	// it, but check if anything still exists within and delete it otherwise.
	if b.f.Type == config.FolderTypeReceiveEncrypted && fi.IsDirectory() && protocol.IsEncryptedParent(fs.PathComponents(fi.Name)) {
		if names, err := b.f.mtimefs.DirNames(fi.Name); err == nil && len(names) == 0 {
			b.f.mtimefs.Remove(fi.Name)
		}
		return false
	}
	// Resolve receive-only items which are identical with the global state or
	// the global item is our own receive-only item.
	switch gf, ok := snap.GetGlobal(fi.Name); {
	case !ok:
	case gf.IsReceiveOnlyChanged():
		if fi.IsDeleted() {
			// Our item is deleted and the global item is our own receive only
			// file. No point in keeping track of that.
			b.Remove(fi.Name)
			return true
		}
	case gf.IsEquivalentOptional(fi, b.f.modTimeWindow, false, false, protocol.FlagLocalReceiveOnly):
		// What we have locally is equivalent to the global file.
		l.Debugf("%v scanning: Merging identical locally changed item with global", b.f, fi)
		fi = gf
	}
	b.updateBatch.Append(fi)
	return true
}

func (f *folder) scanSubdirsChangedAndNew(subDirs []string, batch *scanBatch) (int, error) {
	changes := 0
	snap, err := f.dbSnapshot()
	if err != nil {
		return changes, err
	}
	defer snap.Release()

	// If we return early e.g. due to a folder health error, the scan needs
	// to be cancelled.
	scanCtx, scanCancel := context.WithCancel(f.ctx)
	defer scanCancel()

	scanConfig := scanner.Config{
		Folder:                f.ID,
		Subs:                  subDirs,
		Matcher:               f.ignores,
		TempLifetime:          time.Duration(f.model.cfg.Options().KeepTemporariesH) * time.Hour,
		CurrentFiler:          cFiler{snap},
		Filesystem:            f.mtimefs,
		IgnorePerms:           f.IgnorePerms,
		AutoNormalize:         f.AutoNormalize,
		Hashers:               f.model.numHashers(f.ID),
		ShortID:               f.shortID,
		ProgressTickIntervalS: f.ScanProgressIntervalS,
		LocalFlags:            f.localFlags,
		ModTimeWindow:         f.modTimeWindow,
		EventLogger:           f.evLogger,
	}
	var fchan chan scanner.ScanResult
	if f.Type == config.FolderTypeReceiveEncrypted {
		fchan = scanner.WalkWithoutHashing(scanCtx, scanConfig)
	} else {
		fchan = scanner.Walk(scanCtx, scanConfig)
	}

	alreadyUsedOrExisting := make(map[string]struct{})
	for res := range fchan {
		if res.Err != nil {
			f.newScanError(res.Path, res.Err)
			continue
		}

		if err := batch.FlushIfFull(); err != nil {
			// Prevent a race between the scan aborting due to context
			// cancellation and releasing the snapshot in defer here.
			scanCancel()
			for range fchan {
			}
			return changes, err
		}

		if batch.Update(res.File, snap) {
			changes++
		}

		switch f.Type {
		case config.FolderTypeReceiveOnly, config.FolderTypeReceiveEncrypted:
		default:
			if nf, ok := f.findRename(snap, res.File, alreadyUsedOrExisting); ok {
				if batch.Update(nf, snap) {
					changes++
				}
			}
		}
	}

	return changes, nil
}

func (f *folder) scanSubdirsDeletedAndIgnored(subDirs []string, batch *scanBatch) (int, error) {
	var toIgnore []db.FileInfoTruncated
	ignoredParent := ""
	changes := 0
	snap, err := f.dbSnapshot()
	if err != nil {
		return 0, err
	}
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

			if err := batch.FlushIfFull(); err != nil {
				iterError = err
				return false
			}

			if ignoredParent != "" && !fs.IsParent(file.Name, ignoredParent) {
				for _, file := range toIgnore {
					l.Debugln("marking file as ignored", file)
					nf := file.ConvertToIgnoredFileInfo()
					if batch.Update(nf, snap) {
						changes++
					}
					if err := batch.FlushIfFull(); err != nil {
						iterError = err
						return false
					}
				}
				toIgnore = toIgnore[:0]
				ignoredParent = ""
			}

			switch ignored := f.ignores.Match(file.Name).IsIgnored(); {
			case file.IsIgnored() && ignored:
				return true
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
				nf := file.ConvertToIgnoredFileInfo()
				if batch.Update(nf, snap) {
					changes++
				}

			case file.IsIgnored() && !ignored:
				// Successfully scanned items are already un-ignored during
				// the scan, so check whether it is deleted.
				fallthrough
			case !file.IsIgnored() && !file.IsDeleted() && !file.IsUnsupported():
				// The file is not ignored, deleted or unsupported. Lets check if
				// it's still here. Simply stat:ing it wont do as there are
				// tons of corner cases (e.g. parent dir->symlink, missing
				// permissions)
				if !osutil.IsDeleted(f.mtimefs, file.Name) {
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
				l.Debugln("marking file as deleted", nf)
				if batch.Update(nf, snap) {
					changes++
				}
			case file.IsDeleted() && file.IsReceiveOnlyChanged():
				switch f.Type {
				case config.FolderTypeReceiveOnly, config.FolderTypeReceiveEncrypted:
					switch gf, ok := snap.GetGlobal(file.Name); {
					case !ok:
					case gf.IsReceiveOnlyChanged():
						l.Debugln("removing deleted, receive-only item that is globally receive-only from db", file)
						batch.Remove(file.Name)
						changes++
					case gf.IsDeleted():
						// Our item is deleted and the global item is deleted too. We just
						// pretend it is a normal deleted file (nobody cares about that).
						l.Debugf("%v scanning: Marking globally deleted item as not locally changed: %v", f, file.Name)
						file.LocalFlags &^= protocol.FlagLocalReceiveOnly
						if batch.Update(file.ConvertDeletedToFileInfo(), snap) {
							changes++
						}
					}
				default:
					// No need to bump the version for a file that was and is
					// deleted and just the folder type/local flags changed.
					file.LocalFlags &^= protocol.FlagLocalReceiveOnly
					l.Debugln("removing receive-only flag on deleted item", file)
					if batch.Update(file.ConvertDeletedToFileInfo(), snap) {
						changes++
					}
				}
			}

			return true
		})

		select {
		case <-f.ctx.Done():
			return changes, f.ctx.Err()
		default:
		}

		if iterError == nil && len(toIgnore) > 0 {
			for _, file := range toIgnore {
				l.Debugln("marking file as ignored", f)
				nf := file.ConvertToIgnoredFileInfo()
				if batch.Update(nf, snap) {
					changes++
				}
				if iterError = batch.FlushIfFull(); iterError != nil {
					break
				}
			}
			toIgnore = toIgnore[:0]
		}

		if iterError != nil {
			return changes, iterError
		}
	}

	return changes, nil
}

func (f *folder) findRename(snap *db.Snapshot, file protocol.FileInfo, alreadyUsedOrExisting map[string]struct{}) (protocol.FileInfo, bool) {
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

		if fi.Name == file.Name {
			alreadyUsedOrExisting[fi.Name] = struct{}{}
			return true
		}

		if _, ok := alreadyUsedOrExisting[fi.Name]; ok {
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

		alreadyUsedOrExisting[fi.Name] = struct{}{}

		if !osutil.IsDeleted(f.mtimefs, fi.Name) {
			return true
		}

		nf = fi
		nf.SetDeleted(f.shortID)
		nf.LocalFlags = f.localFlags
		found = true
		return false
	})

	return nf, found
}

func (f *folder) scanTimerFired() error {
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

	return err
}

func (f *folder) versionCleanupTimerFired() {
	f.setState(FolderCleanWaiting)
	defer f.setState(FolderIdle)

	if err := f.ioLimiter.TakeWithContext(f.ctx, 1); err != nil {
		return
	}
	defer f.ioLimiter.Give(1)

	f.setState(FolderCleaning)

	if err := f.versioner.Clean(f.ctx); err != nil {
		l.Infoln("Failed to clean versions in %s: %v", f.Description(), err)
	}

	f.versionCleanupTimer.Reset(f.versionCleanupInterval)
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
func (f *folder) restartWatch() error {
	f.stopWatch()
	f.startWatch()
	return f.scanSubdirs(nil)
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
			var errOutside *fs.ErrWatchEventOutsideRoot
			if errors.As(err, &errOutside) {
				if !warnedOutside {
					l.Warnln(err)
					warnedOutside = true
				}
				f.evLogger.Log(events.Failure, "watching for changes encountered an event outside of the filesystem root")
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
		f.DelayScan(0)
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
		f.SchedulePull()
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
	f.errorsMut.Lock()
	l.Infof("Scanner (folder %s, item %q): %v", f.Description(), path, err)
	f.scanErrors = append(f.scanErrors, FileError{
		Err:  err.Error(),
		Path: path,
	})
	f.errorsMut.Unlock()
}

func (f *folder) clearScanErrors(subDirs []string) {
	f.errorsMut.Lock()
	defer f.errorsMut.Unlock()
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
	f.errorsMut.Lock()
	defer f.errorsMut.Unlock()
	scanLen := len(f.scanErrors)
	errors := make([]FileError, scanLen+len(f.pullErrors))
	copy(errors[:scanLen], f.scanErrors)
	copy(errors[scanLen:], f.pullErrors)
	sort.Sort(fileErrorList(errors))
	return errors
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
	f.forcedRescanPathsMut.Lock()
	for i, file := range fs {
		filenames[i] = file.Name
		// No need to rescan a file that was changed since anyway.
		delete(f.forcedRescanPaths, file.Name)
	}
	f.forcedRescanPathsMut.Unlock()

	seq := f.fset.Sequence(protocol.LocalDeviceID)
	f.evLogger.Log(events.LocalIndexUpdated, map[string]interface{}{
		"folder":    f.ID,
		"items":     len(fs),
		"filenames": filenames,
		"sequence":  seq,
		"version":   seq, // legacy for sequence
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

func (f *folder) handleForcedRescans() error {
	f.forcedRescanPathsMut.Lock()
	paths := make([]string, 0, len(f.forcedRescanPaths))
	for path := range f.forcedRescanPaths {
		paths = append(paths, path)
	}
	f.forcedRescanPaths = make(map[string]struct{})
	f.forcedRescanPathsMut.Unlock()
	if len(paths) == 0 {
		return nil
	}

	batch := db.NewFileInfoBatch(func(fs []protocol.FileInfo) error {
		f.fset.Update(protocol.LocalDeviceID, fs)
		return nil
	})

	snap, err := f.dbSnapshot()
	if err != nil {
		return err
	}
	defer snap.Release()

	for _, path := range paths {
		if err := batch.FlushIfFull(); err != nil {
			return err
		}

		fi, ok := snap.Get(protocol.LocalDeviceID, path)
		if !ok {
			continue
		}
		fi.SetMustRescan()
		batch.Append(fi)
	}

	if err = batch.Flush(); err != nil {
		return err
	}

	return f.scanSubdirs(paths)
}

// dbSnapshots gets a snapshot from the fileset, and wraps any error
// in a svcutil.FatalErr.
func (f *folder) dbSnapshot() (*db.Snapshot, error) {
	snap, err := f.fset.Snapshot()
	if err != nil {
		return nil, svcutil.AsFatalErr(err, svcutil.ExitError)
	}
	return snap, nil
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

// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/internal/slogutil"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
	"github.com/syncthing/syncthing/lib/semaphore"
	"github.com/syncthing/syncthing/lib/versioner"
)

var (
	blockStats    = make(map[string]int)
	blockStatsMut sync.Mutex
)

func init() {
	folderFactories[config.FolderTypeSendReceive] = newSendReceiveFolder
}

// A pullBlockState is passed to the puller routine for each block that needs
// to be fetched.
type pullBlockState struct {
	*sharedPullerState

	block protocol.BlockInfo
}

// A copyBlocksState is passed to copy routine if the file has blocks to be
// copied.
type copyBlocksState struct {
	*sharedPullerState

	blocks []protocol.BlockInfo
	have   int
}

// Which filemode bits to preserve
const retainBits = fs.ModeSetgid | fs.ModeSetuid | fs.ModeSticky

var (
	activity                  = newDeviceActivity()
	errNoDevice               = errors.New("peers who had this file went away, or the file has changed while syncing. will retry later")
	errDirPrefix              = "directory has been deleted on a remote device but "
	errDirHasToBeScanned      = errors.New(errDirPrefix + "contains changed files, scheduling scan")
	errDirHasIgnored          = errors.New(errDirPrefix + "contains ignored files (see ignore documentation for (?d) prefix)")
	errDirNotEmpty            = errors.New(errDirPrefix + "is not empty; the contents are probably ignored on that remote device, but not locally")
	errNotAvailable           = errors.New("no connected device has the required version of this file")
	errModified               = errors.New("file modified but not rescanned; will try again later")
	errUnexpectedDirOnFileDel = errors.New("encountered directory when trying to remove file/symlink")
	errIncompatibleSymlink    = errors.New("incompatible symlink entry; rescan with newer Syncthing on source")
	contextRemovingOldItem    = "removing item to be replaced"
)

type dbUpdateType int

func (d dbUpdateType) String() string {
	switch d {
	case dbUpdateHandleDir:
		return "dbUpdateHandleDir"
	case dbUpdateDeleteDir:
		return "dbUpdateDeleteDir"
	case dbUpdateHandleFile:
		return "dbUpdateHandleFile"
	case dbUpdateDeleteFile:
		return "dbUpdateDeleteFile"
	case dbUpdateShortcutFile:
		return "dbUpdateShortcutFile"
	case dbUpdateHandleSymlink:
		return "dbUpdateHandleSymlink"
	case dbUpdateInvalidate:
		return "dbUpdateHandleInvalidate"
	}
	panic(fmt.Sprintf("unknown dbUpdateType %d", d))
}

const (
	dbUpdateHandleDir dbUpdateType = iota
	dbUpdateDeleteDir
	dbUpdateHandleFile
	dbUpdateDeleteFile
	dbUpdateShortcutFile
	dbUpdateHandleSymlink
	dbUpdateInvalidate
)

const (
	defaultCopiers          = 2
	defaultPullerPause      = 60 * time.Second
	defaultPullerPendingKiB = 2 * protocol.MaxBlockSize / 1024

	maxPullerIterations = 3
)

type dbUpdateJob struct {
	file    protocol.FileInfo
	jobType dbUpdateType
}

type sendReceiveFolder struct {
	*folder

	queue              *jobQueue
	blockPullReorderer blockPullReorderer
	writeLimiter       *semaphore.Semaphore

	tempPullErrors map[string]string // pull errors that might be just transient
}

func newSendReceiveFolder(model *model, ignores *ignore.Matcher, cfg config.FolderConfiguration, ver versioner.Versioner, evLogger events.Logger, ioLimiter *semaphore.Semaphore) service {
	f := &sendReceiveFolder{
		folder:             newFolder(model, ignores, cfg, evLogger, ioLimiter, ver),
		queue:              newJobQueue(),
		blockPullReorderer: newBlockPullReorderer(cfg.BlockPullOrder, model.id, cfg.DeviceIDs()),
		writeLimiter:       semaphore.New(cfg.MaxConcurrentWrites),
	}
	f.puller = f

	if f.Copiers == 0 {
		f.Copiers = defaultCopiers
	}

	// If the configured max amount of pending data is zero, we use the
	// default. If it's configured to something non-zero but less than the
	// protocol block size we adjust it upwards accordingly.
	if f.PullerMaxPendingKiB == 0 {
		f.PullerMaxPendingKiB = defaultPullerPendingKiB
	}
	if blockSizeKiB := protocol.MaxBlockSize / 1024; f.PullerMaxPendingKiB < blockSizeKiB {
		f.PullerMaxPendingKiB = blockSizeKiB
	}

	return f
}

// pull returns true if it manages to get all needed items from peers, i.e. get
// the device in sync with the global state.
func (f *sendReceiveFolder) pull(ctx context.Context) (bool, error) {
	f.sl.DebugContext(ctx, "Pulling")

	scanChan := make(chan string)
	go f.pullScannerRoutine(ctx, scanChan)
	defer func() {
		close(scanChan)
		f.setState(FolderIdle)
	}()

	metricFolderPulls.WithLabelValues(f.ID).Inc()
	pullCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go addTimeUntilCancelled(pullCtx, metricFolderPullSeconds.WithLabelValues(f.ID))

	changed := 0

	f.errorsMut.Lock()
	f.pullErrors = nil
	f.errorsMut.Unlock()

	var err error
	for tries := range maxPullerIterations {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}

		// Needs to be set on every loop, as the puller might have set
		// it to FolderSyncing during the last iteration.
		f.setState(FolderSyncPreparing)

		changed, err = f.pullerIteration(ctx, scanChan)
		if err != nil {
			return false, err
		}

		f.sl.DebugContext(ctx, "Pull iteration completed", "changed", changed, "try", tries+1)

		if changed == 0 {
			// No files were changed by the puller, so we are in sync, or we
			// are unable to make further progress for the moment.
			break
		}
	}

	f.errorsMut.Lock()
	pullErrNum := len(f.tempPullErrors)
	if pullErrNum > 0 {
		f.pullErrors = make([]FileError, 0, len(f.tempPullErrors))
		for path, err := range f.tempPullErrors {
			f.sl.WarnContext(ctx, "Failed to sync", slogutil.FilePath(path), slogutil.Error(err))
			f.pullErrors = append(f.pullErrors, FileError{
				Err:  err,
				Path: path,
			})
		}
		f.tempPullErrors = nil
	}
	f.errorsMut.Unlock()

	if pullErrNum > 0 {
		f.evLogger.Log(events.FolderErrors, map[string]interface{}{
			"folder": f.folderID,
			"errors": f.Errors(),
		})
	}

	// We're done if we didn't change anything and didn't fail to change
	// anything
	return changed == 0 && pullErrNum == 0, nil
}

// pullerIteration runs a single puller iteration for the given folder and
// returns the number items that should have been synced (even those that
// might have failed). One puller iteration handles all files currently
// flagged as needed in the folder.
func (f *sendReceiveFolder) pullerIteration(ctx context.Context, scanChan chan<- string) (int, error) {
	f.errorsMut.Lock()
	f.tempPullErrors = make(map[string]string)
	f.errorsMut.Unlock()

	pullChan := make(chan pullBlockState)
	copyChan := make(chan copyBlocksState)
	finisherChan := make(chan *sharedPullerState)
	dbUpdateChan := make(chan dbUpdateJob)

	var pullWg sync.WaitGroup
	var copyWg sync.WaitGroup
	var doneWg sync.WaitGroup
	var updateWg sync.WaitGroup

	f.sl.DebugContext(ctx, "Starting puller iteration", "copiers", f.Copiers, "pullerPendingKiB", f.PullerMaxPendingKiB)

	updateWg.Add(1)
	var changed int // only read after updateWg closes
	go func() {
		// dbUpdaterRoutine finishes when dbUpdateChan is closed
		changed = f.dbUpdaterRoutine(dbUpdateChan)
		updateWg.Done()
	}()

	for range f.Copiers {
		copyWg.Add(1)
		go func() {
			// copierRoutine finishes when copyChan is closed
			f.copierRoutine(ctx, copyChan, pullChan, finisherChan)
			copyWg.Done()
		}()
	}

	pullWg.Add(1)
	go func() {
		// pullerRoutine finishes when pullChan is closed
		f.pullerRoutine(ctx, pullChan, finisherChan)
		pullWg.Done()
	}()

	doneWg.Add(1)
	// finisherRoutine finishes when finisherChan is closed
	go func() {
		f.finisherRoutine(ctx, finisherChan, dbUpdateChan, scanChan)
		doneWg.Done()
	}()

	fileDeletions, dirDeletions, err := f.processNeeded(ctx, dbUpdateChan, copyChan, scanChan)

	// Signal copy and puller routines that we are done with the in data for
	// this iteration. Wait for them to finish.
	close(copyChan)
	copyWg.Wait()
	close(pullChan)
	pullWg.Wait()

	// Signal the finisher chan that there will be no more input and wait
	// for it to finish.
	close(finisherChan)
	doneWg.Wait()

	if err == nil {
		f.processDeletions(ctx, fileDeletions, dirDeletions, dbUpdateChan, scanChan)
	}

	// Wait for db updates and scan scheduling to complete
	close(dbUpdateChan)
	updateWg.Wait()

	f.queue.Reset()

	return changed, err
}

func (f *sendReceiveFolder) processNeeded(ctx context.Context, dbUpdateChan chan<- dbUpdateJob, copyChan chan<- copyBlocksState, scanChan chan<- string) (map[string]protocol.FileInfo, []protocol.FileInfo, error) {
	var dirDeletions []protocol.FileInfo
	fileDeletions := map[string]protocol.FileInfo{}
	buckets := map[string][]protocol.FileInfo{}

	// Iterate the list of items that we need and sort them into piles.
	// Regular files to pull goes into the file queue, everything else
	// (directories, symlinks and deletes) goes into the "process directly"
	// pile.
loop:
	for file, err := range itererr.Zip(f.model.sdb.AllNeededGlobalFiles(f.folderID, protocol.LocalDeviceID, f.Order, 0, 0)) {
		if err != nil {
			return nil, nil, err
		}
		select {
		case <-ctx.Done():
			break loop
		default:
		}

		if f.IgnoreDelete && file.IsDeleted() {
			f.sl.DebugContext(ctx, "Ignoring file deletion per config", slogutil.FilePath(file.FileName()))
			continue
		}

		switch {
		case f.ignores.Match(file.Name).IsIgnored():
			file.SetIgnored()
			f.sl.DebugContext(ctx, "Handling ignored file", file.LogAttr())
			dbUpdateChan <- dbUpdateJob{file, dbUpdateInvalidate}

		case build.IsWindows && fs.WindowsInvalidFilename(file.Name) != nil:
			if file.IsDeleted() {
				// Just pretend we deleted it, no reason to create an error
				// about a deleted file that we can't have anyway.
				// Reason we need it in the first place is, that it was
				// ignored at some point.
				dbUpdateChan <- dbUpdateJob{file, dbUpdateDeleteFile}
			} else {
				// We can't pull an invalid file. Grab the error again since
				// we couldn't assign it directly in the case clause.
				f.newPullError(file.Name, fs.WindowsInvalidFilename(file.Name))
			}

		case file.IsDeleted():
			switch {
			case file.IsDirectory():
				// Perform directory deletions at the end, as we may have
				// files to delete inside them before we get to that point.
				dirDeletions = append(dirDeletions, file)
			case file.IsSymlink():
				f.deleteFile(file, dbUpdateChan, scanChan)
			default:
				df, ok, err := f.model.sdb.GetDeviceFile(f.folderID, protocol.LocalDeviceID, file.Name)
				if err != nil {
					return nil, nil, err
				}
				// Local file can be already deleted, but with a lower version
				// number, hence the deletion coming in again as part of
				// WithNeed, furthermore, the file can simply be of the wrong
				// type if we haven't yet managed to pull it.
				if ok && !df.IsDeleted() && !df.IsSymlink() && !df.IsDirectory() && !df.IsInvalid() {
					fileDeletions[file.Name] = file
					// Put files into buckets per first hash
					key := string(df.BlocksHash)
					buckets[key] = append(buckets[key], df)
				} else {
					f.deleteFileWithCurrent(file, df, ok, dbUpdateChan, scanChan)
				}
			}

		case file.Type == protocol.FileInfoTypeFile:
			curFile, hasCurFile, err := f.model.sdb.GetDeviceFile(f.folderID, protocol.LocalDeviceID, file.Name)
			if err != nil {
				return nil, nil, err
			}
			if hasCurFile && file.BlocksEqual(curFile) {
				// We are supposed to copy the entire file, and then fetch nothing. We
				// are only updating metadata, so we don't actually *need* to make the
				// copy.
				f.shortcutFile(file, dbUpdateChan)
			} else {
				// Queue files for processing after directories and symlinks.
				f.queue.Push(file.Name, file.Size, file.ModTime())
			}

		case (build.IsWindows || build.IsAndroid) && file.IsSymlink():
			if err := f.handleSymlinkCheckExisting(file, scanChan); err != nil {
				f.newPullError(file.Name, fmt.Errorf("handling unsupported symlink: %w", err))
				break
			}
			file.SetUnsupported()
			f.sl.DebugContext(ctx, "Invalidating unsupported symlink", slogutil.FilePath(file.Name))
			dbUpdateChan <- dbUpdateJob{file, dbUpdateInvalidate}

		case file.IsDirectory() && !file.IsSymlink():
			f.sl.DebugContext(ctx, "Handling directory", slogutil.FilePath(file.Name))
			if f.checkParent(file.Name, scanChan) {
				f.handleDir(file, dbUpdateChan, scanChan)
			}

		case file.IsSymlink():
			f.sl.DebugContext(ctx, "Handling symlink", slogutil.FilePath(file.Name))
			if f.checkParent(file.Name, scanChan) {
				f.handleSymlink(file, dbUpdateChan, scanChan)
			}

		default:
			panic("unhandleable item type, can't happen")
		}
	}

	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	default:
	}

	// Process the file queue.

nextFile:
	for {
		select {
		case <-ctx.Done():
			return fileDeletions, dirDeletions, ctx.Err()
		default:
		}

		fileName, ok := f.queue.Pop()
		if !ok {
			break
		}

		fi, ok, err := f.model.sdb.GetGlobalFile(f.folderID, fileName)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			// File is no longer in the index. Mark it as done and drop it.
			f.queue.Done(fileName)
			continue
		}

		if fi.IsDeleted() || fi.IsInvalid() || fi.Type != protocol.FileInfoTypeFile {
			// The item has changed type or status in the index while we
			// were processing directories above.
			f.queue.Done(fileName)
			continue
		}

		if !f.checkParent(fi.Name, scanChan) {
			f.queue.Done(fileName)
			continue
		}

		// Check our list of files to be removed for a match, in which case
		// we can just do a rename instead.
		key := string(fi.BlocksHash)
		for candidate, ok := popCandidate(buckets, key); ok; candidate, ok = popCandidate(buckets, key) {
			// candidate is our current state of the file, where as the
			// desired state with the delete bit set is in the deletion
			// map.
			desired := fileDeletions[candidate.Name]
			if err := f.renameFile(candidate, desired, fi, dbUpdateChan, scanChan); err != nil {
				f.sl.DebugContext(ctx, "Rename shortcut failed", slogutil.FilePath(fi.Name), slogutil.Error(err))
				// Failed to rename, try next one.
				continue
			}

			// Remove the pending deletion (as we performed it by renaming)
			delete(fileDeletions, candidate.Name)

			f.queue.Done(fileName)
			continue nextFile
		}

		// Verify there is some availability for the file before we start
		// processing it
		devices := f.model.fileAvailability(f.FolderConfiguration, fi)
		if len(devices) == 0 {
			f.newPullError(fileName, errNotAvailable)
			f.queue.Done(fileName)
			continue
		}

		// Verify we have space to handle the file before we start
		// creating temp files etc.
		if err := f.CheckAvailableSpace(uint64(fi.Size)); err != nil { //nolint:gosec
			f.newPullError(fileName, err)
			f.queue.Done(fileName)
			continue
		}

		if err := f.handleFile(ctx, fi, copyChan); err != nil {
			f.newPullError(fileName, err)
		}
	}

	return fileDeletions, dirDeletions, nil
}

func popCandidate(buckets map[string][]protocol.FileInfo, key string) (protocol.FileInfo, bool) {
	cands := buckets[key]
	if len(cands) == 0 {
		return protocol.FileInfo{}, false
	}

	buckets[key] = cands[1:]
	return cands[0], true
}

func (f *sendReceiveFolder) processDeletions(ctx context.Context, fileDeletions map[string]protocol.FileInfo, dirDeletions []protocol.FileInfo, dbUpdateChan chan<- dbUpdateJob, scanChan chan<- string) {
	for _, file := range fileDeletions {
		select {
		case <-ctx.Done():
			return
		default:
		}

		f.deleteFile(file, dbUpdateChan, scanChan)
	}

	// Process in reverse order to delete depth first
	for i := range dirDeletions {
		select {
		case <-ctx.Done():
			return
		default:
		}

		dir := dirDeletions[len(dirDeletions)-i-1]
		f.sl.DebugContext(ctx, "Deleting directory", slogutil.FilePath(dir.Name))
		f.deleteDir(dir, dbUpdateChan, scanChan)
	}
}

// handleDir creates or updates the given directory
func (f *sendReceiveFolder) handleDir(file protocol.FileInfo, dbUpdateChan chan<- dbUpdateJob, scanChan chan<- string) {
	// Used in the defer closure below, updated by the function body. Take
	// care not declare another err.
	var err error

	f.evLogger.Log(events.ItemStarted, map[string]string{
		"folder": f.folderID,
		"item":   file.Name,
		"type":   "dir",
		"action": "update",
	})

	defer func() {
		slog.Info("Created or updated directory", f.LogAttr(), file.LogAttr())
		f.evLogger.Log(events.ItemFinished, map[string]interface{}{
			"folder": f.folderID,
			"item":   file.Name,
			"error":  events.Error(err),
			"type":   "dir",
			"action": "update",
		})
	}()

	mode := fs.FileMode(file.Permissions & 0o777)
	if f.IgnorePerms || file.NoPermissions {
		mode = 0o777
	}

	f.sl.Debug("Need dir", "file", file, "cur", slogutil.Expensive(func() any {
		curFile, _, _ := f.model.sdb.GetDeviceFile(f.folderID, protocol.LocalDeviceID, file.Name)
		return curFile
	}))
	info, err := f.mtimefs.Lstat(file.Name)
	switch {
	// There is already something under that name, we need to handle that.
	// Unless it already is a directory, as we only track permissions,
	// that don't result in a conflict.
	case err == nil && !info.IsDir():
		// Check that it is what we have in the database.
		curFile, hasCurFile, err := f.model.sdb.GetDeviceFile(f.folderID, protocol.LocalDeviceID, file.Name)
		if err != nil {
			f.newPullError(file.Name, fmt.Errorf("handling dir: %w", err))
			return
		}
		if err := f.scanIfItemChanged(file.Name, info, curFile, hasCurFile, false, scanChan); err != nil {
			f.newPullError(file.Name, fmt.Errorf("handling dir: %w", err))
			return
		}

		// Remove it to replace with the dir.
		if !curFile.IsSymlink() && file.InConflictWith(curFile) {
			// The new file has been changed in conflict with the existing one. We
			// should file it away as a conflict instead of just removing or
			// archiving.
			// Symlinks aren't checked for conflicts.

			err = f.inWritableDir(func(name string) error {
				return f.moveForConflict(name, file.ModifiedBy.String(), scanChan)
			}, curFile.Name)
		} else {
			err = f.deleteItemOnDisk(curFile, scanChan)
		}
		if err != nil {
			f.newPullError(file.Name, err)
			return
		}
		fallthrough
	// The directory doesn't exist, so we create it with the right
	// mode bits from the start.
	case err != nil && fs.IsNotExist(err):
		// We declare a function that acts on only the path name, so
		// we can pass it to InWritableDir. We use a regular Mkdir and
		// not MkdirAll because the parent should already exist.
		mkdir := func(path string) error {
			err = f.mtimefs.Mkdir(path, mode)
			if err != nil {
				return err
			}

			// Set the platform data (ownership, xattrs, etc).
			if err := f.setPlatformData(&file, path); err != nil {
				return err
			}

			if f.IgnorePerms || file.NoPermissions {
				return nil
			}

			// Stat the directory so we can check its permissions.
			info, err := f.mtimefs.Lstat(path)
			if err != nil {
				return err
			}

			// Mask for the bits we want to preserve and add them in to the
			// directories permissions.
			return f.mtimefs.Chmod(path, mode|(info.Mode()&retainBits))
		}

		if err = f.inWritableDir(mkdir, file.Name); err == nil {
			dbUpdateChan <- dbUpdateJob{file, dbUpdateHandleDir}
		} else {
			f.newPullError(file.Name, fmt.Errorf("creating directory: %w", err))
		}
		return
	// Weird error when stat()'ing the dir. Probably won't work to do
	// anything else with it if we can't even stat() it.
	case err != nil:
		f.newPullError(file.Name, fmt.Errorf("checking file to be replaced: %w", err))
		return
	}

	// The directory already exists, so we just correct the metadata. (We
	// don't handle modification times on directories, because that sucks...)
	// It's OK to change mode bits on stuff within non-writable directories.
	if !f.IgnorePerms && !file.NoPermissions {
		if err := f.mtimefs.Chmod(file.Name, mode|(info.Mode()&retainBits)); err != nil {
			f.newPullError(file.Name, fmt.Errorf("handling dir (setting permissions): %w", err))
			return
		}
		if err := f.setPlatformData(&file, file.Name); err != nil {
			f.newPullError(file.Name, fmt.Errorf("handling dir (setting metadata): %w", err))
			return
		}
	}
	dbUpdateChan <- dbUpdateJob{file, dbUpdateHandleDir}
}

// checkParent verifies that the thing we are handling lives inside a directory,
// and not a symlink or regular file. It also resurrects missing parent dirs.
func (f *sendReceiveFolder) checkParent(file string, scanChan chan<- string) bool {
	parent := filepath.Dir(file)

	if err := osutil.TraversesSymlink(f.mtimefs, parent); err != nil {
		f.newPullError(file, fmt.Errorf("checking parent dirs: %w", err))
		return false
	}

	// issues #114 and #4475: This works around a race condition
	// between two devices, when one device removes a directory and the
	// other creates a file in it. However that happens, we end up with
	// a directory for "foo" with the delete bit, but a file "foo/bar"
	// that we want to sync. We never create the directory, and hence
	// fail to create the file and end up looping forever on it. This
	// breaks that by creating the directory and scheduling a scan,
	// where it will be found and the delete bit on it removed. The
	// user can then clean up as they like...
	// This can also occur if an entire tree structure was deleted, but only
	// a leave has been scanned.
	//
	// And if this is an encrypted folder:
	// Encrypted files have made-up filenames with two synthetic parent
	// directories which don't have any meaning. Create those if necessary.
	if _, err := f.mtimefs.Lstat(parent); !fs.IsNotExist(err) {
		f.sl.Debug("Parent directory exists", slogutil.FilePath(file))
		return true
	}
	f.sl.Debug("Creating parent directory", slogutil.FilePath(file))
	if err := f.mtimefs.MkdirAll(parent, 0o755); err != nil {
		f.newPullError(file, fmt.Errorf("creating parent dir: %w", err))
		return false
	}
	if f.Type != config.FolderTypeReceiveEncrypted {
		scanChan <- parent
	}
	return true
}

// handleSymlink creates or updates the given symlink
func (f *sendReceiveFolder) handleSymlink(file protocol.FileInfo, dbUpdateChan chan<- dbUpdateJob, scanChan chan<- string) {
	// Used in the defer closure below, updated by the function body. Take
	// care not declare another err.
	var err error

	f.evLogger.Log(events.ItemStarted, map[string]string{
		"folder": f.folderID,
		"item":   file.Name,
		"type":   "symlink",
		"action": "update",
	})

	defer func() {
		if err != nil {
			slog.Warn("Failed to handle symlink", f.LogAttr(), file.LogAttr(), slogutil.Error(err))
		} else {
			slog.Info("Created or updated symlink", f.LogAttr(), file.LogAttr())
		}
		f.evLogger.Log(events.ItemFinished, map[string]interface{}{
			"folder": f.folderID,
			"item":   file.Name,
			"error":  events.Error(err),
			"type":   "symlink",
			"action": "update",
		})
	}()

	f.sl.Debug("Need symlink", slogutil.FilePath(file.Name), slog.Any("cur", slogutil.Expensive(func() any {
		curFile, _, _ := f.model.sdb.GetDeviceFile(f.folderID, protocol.LocalDeviceID, file.Name)
		return curFile
	})))

	if len(file.SymlinkTarget) == 0 {
		// Index entry from a Syncthing predating the support for including
		// the link target in the index entry. We log this as an error.
		f.newPullError(file.Name, errIncompatibleSymlink)
		return
	}

	if err = f.handleSymlinkCheckExisting(file, scanChan); err != nil {
		f.newPullError(file.Name, fmt.Errorf("handling symlink: %w", err))
		return
	}

	// We declare a function that acts on only the path name, so
	// we can pass it to InWritableDir.
	createLink := func(path string) error {
		if err := f.mtimefs.CreateSymlink(string(file.SymlinkTarget), path); err != nil {
			return err
		}
		return f.setPlatformData(&file, path)
	}

	if err = f.inWritableDir(createLink, file.Name); err == nil {
		dbUpdateChan <- dbUpdateJob{file, dbUpdateHandleSymlink}
	} else {
		f.newPullError(file.Name, fmt.Errorf("symlink create: %w", err))
	}
}

func (f *sendReceiveFolder) handleSymlinkCheckExisting(file protocol.FileInfo, scanChan chan<- string) error {
	// If there is already something under that name, we need to handle that.
	info, err := f.mtimefs.Lstat(file.Name)
	if err != nil {
		if fs.IsNotExist(err) {
			return nil
		}
		return err
	}
	// Check that it is what we have in the database.
	curFile, hasCurFile, err := f.model.sdb.GetDeviceFile(f.folderID, protocol.LocalDeviceID, file.Name)
	if err != nil {
		return err
	}
	if err := f.scanIfItemChanged(file.Name, info, curFile, hasCurFile, false, scanChan); err != nil {
		return err
	}
	// Remove it to replace with the symlink. This also handles the
	// "change symlink type" path.
	if !curFile.IsDirectory() && !curFile.IsSymlink() && file.InConflictWith(curFile) {
		// The new file has been changed in conflict with the existing one. We
		// should file it away as a conflict instead of just removing or
		// archiving.
		// Directories and symlinks aren't checked for conflicts.

		return f.inWritableDir(func(name string) error {
			return f.moveForConflict(name, file.ModifiedBy.String(), scanChan)
		}, curFile.Name)
	} else {
		return f.deleteItemOnDisk(curFile, scanChan)
	}
}

// deleteDir attempts to remove a directory that was deleted on a remote
func (f *sendReceiveFolder) deleteDir(file protocol.FileInfo, dbUpdateChan chan<- dbUpdateJob, scanChan chan<- string) {
	// Used in the defer closure below, updated by the function body. Take
	// care not declare another err.
	var err error

	f.evLogger.Log(events.ItemStarted, map[string]string{
		"folder": f.folderID,
		"item":   file.Name,
		"type":   "dir",
		"action": "delete",
	})

	defer func() {
		if err != nil {
			f.newPullError(file.Name, fmt.Errorf("delete dir: %w", err))
			slog.Info("Failed to delete directory", f.LogAttr(), file.LogAttr(), slogutil.Error(err))
		} else {
			slog.Info("Deleted directory", f.LogAttr(), file.LogAttr())
		}
		f.evLogger.Log(events.ItemFinished, map[string]interface{}{
			"folder": f.folderID,
			"item":   file.Name,
			"error":  events.Error(err),
			"type":   "dir",
			"action": "delete",
		})
	}()

	cur, hasCur, err := f.model.sdb.GetDeviceFile(f.folderID, protocol.LocalDeviceID, file.Name)
	if err != nil {
		return
	}

	if err = f.checkToBeDeleted(file, cur, hasCur, scanChan); err != nil {
		if fs.IsNotExist(err) || fs.IsErrCaseConflict(err) {
			err = nil
			dbUpdateChan <- dbUpdateJob{file, dbUpdateDeleteDir}
		}
		return
	}

	if err = f.deleteDirOnDisk(file.Name, scanChan); err != nil {
		return
	}

	dbUpdateChan <- dbUpdateJob{file, dbUpdateDeleteDir}
}

// deleteFile attempts to delete the given file
func (f *sendReceiveFolder) deleteFile(file protocol.FileInfo, dbUpdateChan chan<- dbUpdateJob, scanChan chan<- string) {
	cur, hasCur, err := f.model.sdb.GetDeviceFile(f.folderID, protocol.LocalDeviceID, file.Name)
	if err != nil {
		f.newPullError(file.Name, fmt.Errorf("delete file: %w", err))
		return
	}
	f.deleteFileWithCurrent(file, cur, hasCur, dbUpdateChan, scanChan)
}

func (f *sendReceiveFolder) deleteFileWithCurrent(file, cur protocol.FileInfo, hasCur bool, dbUpdateChan chan<- dbUpdateJob, scanChan chan<- string) {
	// Used in the defer closure below, updated by the function body. Take
	// care not declare another err.
	var err error

	f.sl.Debug("Deleting file or symlink", slogutil.FilePath(file.Name))

	f.evLogger.Log(events.ItemStarted, map[string]string{
		"folder": f.folderID,
		"item":   file.Name,
		"type":   "file",
		"action": "delete",
	})

	defer func() {
		kind := "file"
		if file.IsSymlink() {
			kind = "symlink"
		}
		if err != nil {
			f.newPullError(file.Name, fmt.Errorf("delete file: %w", err))
			slog.Info("Failed to delete "+kind, f.LogAttr(), file.LogAttr(), slogutil.Error(err))
		} else {
			slog.Info("Deleted "+kind, f.LogAttr(), file.LogAttr())
		}
		f.evLogger.Log(events.ItemFinished, map[string]interface{}{
			"folder": f.folderID,
			"item":   file.Name,
			"error":  events.Error(err),
			"type":   "file",
			"action": "delete",
		})
	}()

	if err = f.checkToBeDeleted(file, cur, hasCur, scanChan); err != nil {
		if fs.IsNotExist(err) || fs.IsErrCaseConflict(err) {
			err = nil
			dbUpdateChan <- dbUpdateJob{file, dbUpdateDeleteFile}
		}
		return
	}

	// We are asked to delete a file, but what we have on disk and in db
	// is a directory. Something is wrong here, should probably not happen.
	if cur.IsDirectory() {
		err = errUnexpectedDirOnFileDel
		return
	}

	switch {
	case file.InConflictWith(cur) && !cur.IsSymlink():
		// If the delete constitutes winning a conflict, we move the file to
		// a conflict copy instead of doing the delete
		err = f.inWritableDir(func(name string) error {
			return f.moveForConflict(name, file.ModifiedBy.String(), scanChan)
		}, cur.Name)

	case f.versioner != nil && !cur.IsSymlink():
		// If we have a versioner, use that to move the file away
		err = f.inWritableDir(f.versioner.Archive, file.Name)

	default:
		// Delete the file
		err = f.inWritableDir(f.mtimefs.Remove, file.Name)
	}

	if err == nil || fs.IsNotExist(err) {
		// It was removed or it doesn't exist to start with
		dbUpdateChan <- dbUpdateJob{file, dbUpdateDeleteFile}
		return
	}

	if _, serr := f.mtimefs.Lstat(file.Name); serr != nil && !fs.IsPermission(serr) {
		// We get an error just looking at the file, and it's not a permission
		// problem. Lets assume the error is in fact some variant of "file
		// does not exist" (possibly expressed as some parent being a file and
		// not a directory etc) and that the delete is handled.
		err = nil
		dbUpdateChan <- dbUpdateJob{file, dbUpdateDeleteFile}
	}

	slog.Info("Deleted file", f.LogAttr(), file.LogAttr())
}

// renameFile attempts to rename an existing file to a destination
// and set the right attributes on it.
func (f *sendReceiveFolder) renameFile(cur, source, target protocol.FileInfo, dbUpdateChan chan<- dbUpdateJob, scanChan chan<- string) error {
	// Used in the defer closure below, updated by the function body. Take
	// care not declare another err.
	var err error

	f.evLogger.Log(events.ItemStarted, map[string]string{
		"folder": f.folderID,
		"item":   source.Name,
		"type":   "file",
		"action": "delete",
	})
	f.evLogger.Log(events.ItemStarted, map[string]string{
		"folder": f.folderID,
		"item":   target.Name,
		"type":   "file",
		"action": "update",
	})

	defer func() {
		if err != nil {
			slog.Info("Failed to rename file", f.LogAttr(), target.LogAttr(), slog.String("from", source.Name), slogutil.Error(err))
		} else {
			slog.Info("Renamed file", f.LogAttr(), target.LogAttr(), slog.String("from", source.Name))
		}
		f.evLogger.Log(events.ItemFinished, map[string]interface{}{
			"folder": f.folderID,
			"item":   source.Name,
			"error":  events.Error(err),
			"type":   "file",
			"action": "delete",
		})
		f.evLogger.Log(events.ItemFinished, map[string]interface{}{
			"folder": f.folderID,
			"item":   target.Name,
			"error":  events.Error(err),
			"type":   "file",
			"action": "update",
		})
	}()

	f.sl.Debug("Taking rename shortcut", "from", source.Name, "to", target.Name)

	// Check that source is compatible with what we have in the DB
	if err = f.checkToBeDeleted(source, cur, true, scanChan); err != nil {
		return err
	}
	// Check that the target corresponds to what we have in the DB
	curTarget, ok, err := f.model.sdb.GetDeviceFile(f.folderID, protocol.LocalDeviceID, target.Name)
	if err != nil {
		return err
	}
	switch stat, serr := f.mtimefs.Lstat(target.Name); {
	case serr != nil:
		var caseErr *fs.CaseConflictError
		switch {
		case errors.As(serr, &caseErr):
			if caseErr.Real != source.Name {
				err = serr
				break
			}
			fallthrough // This is a case only rename
		case fs.IsNotExist(serr):
			if !ok || curTarget.IsDeleted() {
				break
			}
			scanChan <- target.Name
			err = errModified
		default:
			// We can't check whether the file changed as compared to the db,
			// do not delete.
			err = serr
		}
	case !ok:
		// Target appeared from nowhere
		scanChan <- target.Name
		err = errModified
	default:
		var fi protocol.FileInfo
		if fi, err = scanner.CreateFileInfo(stat, target.Name, f.mtimefs, f.SyncOwnership, f.SyncXattrs, f.XattrFilter); err == nil {
			if !fi.IsEquivalentOptional(curTarget, protocol.FileInfoComparison{
				ModTimeWindow:   f.modTimeWindow,
				IgnorePerms:     f.IgnorePerms,
				IgnoreBlocks:    true,
				IgnoreFlags:     protocol.LocalAllFlags,
				IgnoreOwnership: !f.SyncOwnership,
				IgnoreXattrs:    !f.SyncXattrs,
			}) {
				// Target changed
				scanChan <- target.Name
				err = errModified
			}
		}
	}
	if err != nil {
		return err
	}

	tempName := fs.TempName(target.Name)

	if f.versioner != nil {
		err = f.CheckAvailableSpace(uint64(source.Size)) //nolint:gosec
		if err == nil {
			err = osutil.Copy(f.CopyRangeMethod.ToFS(), f.mtimefs, f.mtimefs, source.Name, tempName)
			if err == nil {
				err = f.inWritableDir(f.versioner.Archive, source.Name)
			}
		}
	} else {
		err = osutil.RenameOrCopy(f.CopyRangeMethod.ToFS(), f.mtimefs, f.mtimefs, source.Name, tempName)
	}
	if err != nil {
		return err
	}

	blockStatsMut.Lock()
	minBlocksPerBlock := target.BlockSize() / protocol.MinBlockSize
	blockStats["total"] += len(target.Blocks) * minBlocksPerBlock
	blockStats["renamed"] += len(target.Blocks) * minBlocksPerBlock
	blockStatsMut.Unlock()

	// The file was renamed, so we have handled both the necessary delete
	// of the source and the creation of the target temp file. Fix-up the metadata,
	// update the local index of the target file and rename from temp to real name.

	if err = f.performFinish(target, curTarget, true, tempName, dbUpdateChan, scanChan); err != nil {
		return err
	}

	dbUpdateChan <- dbUpdateJob{source, dbUpdateDeleteFile}

	return nil
}

// This is the flow of data and events here, I think...
//
// +-----------------------+
// |                       | - - - - > ItemStarted
// |      handleFile       | - - - - > ItemFinished (on shortcuts)
// |                       |
// +-----------------------+
//             |
//             | copyChan (copyBlocksState; unless shortcut taken)
//             |
//             |    +-----------------------+
//             |    | +-----------------------+
//             +--->| |                       |
//                  | |     copierRoutine     |
//                  +-|                       |
//                    +-----------------------+
//                                |
//                                | pullChan (sharedPullerState)
//                                |
//                                |   +-----------------------+
//                                |   | +-----------------------+
//                                +-->| |                       |
//                                    | |     pullerRoutine     |
//                                    +-|                       |
//                                      +-----------------------+
//                                                  |
//                                                  | finisherChan (sharedPullerState)
//                                                  |
//                                                  |   +-----------------------+
//                                                  |   |                       |
//                                                  +-->|    finisherRoutine    | - - - - > ItemFinished
//                                                      |                       |
//                                                      +-----------------------+

// handleFile queues the copies and pulls as necessary for a single new or
// changed file.
func (f *sendReceiveFolder) handleFile(ctx context.Context, file protocol.FileInfo, copyChan chan<- copyBlocksState) error {
	curFile, hasCurFile, err := f.model.sdb.GetDeviceFile(f.folderID, protocol.LocalDeviceID, file.Name)
	if err != nil {
		return err
	}

	have, _ := blockDiff(curFile.Blocks, file.Blocks)

	tempName := fs.TempName(file.Name)

	populateOffsets(file.Blocks)

	blocks := append([]protocol.BlockInfo{}, file.Blocks...)
	reused := make([]int, 0, len(file.Blocks))

	if f.Type != config.FolderTypeReceiveEncrypted {
		blocks, reused = f.reuseBlocks(ctx, blocks, reused, file, tempName)
	}

	// The sharedpullerstate will know which flags to use when opening the
	// temp file depending if we are reusing any blocks or not.
	if len(reused) == 0 {
		// Otherwise, discard the file ourselves in order for the
		// sharedpuller not to panic when it fails to exclusively create a
		// file which already exists
		f.inWritableDir(f.mtimefs.Remove, tempName)
	}

	// Reorder blocks
	blocks = f.blockPullReorderer.Reorder(blocks)

	f.evLogger.Log(events.ItemStarted, map[string]string{
		"folder": f.folderID,
		"item":   file.Name,
		"type":   "file",
		"action": "update",
	})

	s := newSharedPullerState(file, f.mtimefs, f.folderID, tempName, blocks, reused, f.IgnorePerms || file.NoPermissions, hasCurFile, curFile, !f.DisableSparseFiles, !f.DisableFsync)

	f.sl.DebugContext(ctx, "Handling file", slogutil.FilePath(file.Name), "blocksToCopy", len(blocks), "reused", len(reused))

	cs := copyBlocksState{
		sharedPullerState: s,
		blocks:            blocks,
		have:              len(have),
	}
	copyChan <- cs
	return nil
}

func (f *sendReceiveFolder) reuseBlocks(ctx context.Context, blocks []protocol.BlockInfo, reused []int, file protocol.FileInfo, tempName string) ([]protocol.BlockInfo, []int) {
	// Check for an old temporary file which might have some blocks we could
	// reuse.
	tempBlocks, err := scanner.HashFile(ctx, f.ID, f.mtimefs, tempName, file.BlockSize(), nil)
	if err != nil {
		var caseErr *fs.CaseConflictError
		if errors.As(err, &caseErr) {
			if rerr := f.mtimefs.Rename(caseErr.Real, tempName); rerr == nil {
				tempBlocks, err = scanner.HashFile(ctx, f.ID, f.mtimefs, tempName, file.BlockSize(), nil)
			}
		}
	}
	if err != nil {
		return blocks, reused
	}

	// Check for any reusable blocks in the temp file
	tempCopyBlocks, _ := blockDiff(tempBlocks, file.Blocks)

	// block.String() returns a string unique to the block
	existingBlocks := make(map[string]struct{}, len(tempCopyBlocks))
	for _, block := range tempCopyBlocks {
		existingBlocks[block.String()] = struct{}{}
	}

	// Since the blocks are already there, we don't need to get them.
	blocks = blocks[:0]
	for i, block := range file.Blocks {
		_, ok := existingBlocks[block.String()]
		if !ok {
			blocks = append(blocks, block)
		} else {
			reused = append(reused, i)
		}
	}

	return blocks, reused
}

// blockDiff returns lists of common and missing (to transform src into tgt)
// blocks. Both block lists must have been created with the same block size.
func blockDiff(src, tgt []protocol.BlockInfo) ([]protocol.BlockInfo, []protocol.BlockInfo) {
	if len(tgt) == 0 {
		return nil, nil
	}

	if len(src) == 0 {
		// Copy the entire file
		return nil, tgt
	}

	have := make([]protocol.BlockInfo, 0, len(src))
	need := make([]protocol.BlockInfo, 0, len(tgt))

	for i := range tgt {
		if i >= len(src) {
			return have, append(need, tgt[i:]...)
		}
		if !bytes.Equal(tgt[i].Hash, src[i].Hash) {
			// Copy differing block
			need = append(need, tgt[i])
		} else {
			have = append(have, tgt[i])
		}
	}

	return have, need
}

// populateOffsets sets the Offset field on each block
func populateOffsets(blocks []protocol.BlockInfo) {
	var offset int64
	for i := range blocks {
		blocks[i].Offset = offset
		offset += int64(blocks[i].Size)
	}
}

// shortcutFile sets file metadata, when that's the only thing that has
// changed.
func (f *sendReceiveFolder) shortcutFile(file protocol.FileInfo, dbUpdateChan chan<- dbUpdateJob) {
	f.sl.Debug("Taking metadata shortcut", slogutil.FilePath(file.Name))

	f.evLogger.Log(events.ItemStarted, map[string]string{
		"folder": f.folderID,
		"item":   file.Name,
		"type":   "file",
		"action": "metadata",
	})

	var err error
	defer func() {
		if err != nil {
			slog.Info("Failed to update file metadata", f.LogAttr(), file.LogAttr(), slogutil.Error(err))
		} else {
			slog.Info("Updated file metadata", f.LogAttr(), file.LogAttr())
		}
		f.evLogger.Log(events.ItemFinished, map[string]interface{}{
			"folder": f.folderID,
			"item":   file.Name,
			"error":  events.Error(err),
			"type":   "file",
			"action": "metadata",
		})
	}()

	f.queue.Done(file.Name)

	if !f.IgnorePerms && !file.NoPermissions {
		if err = f.mtimefs.Chmod(file.Name, fs.FileMode(file.Permissions&0o777)); err != nil {
			f.newPullError(file.Name, fmt.Errorf("shortcut file (setting permissions): %w", err))
			return
		}
	}

	if err := f.setPlatformData(&file, file.Name); err != nil {
		f.newPullError(file.Name, fmt.Errorf("shortcut file (setting metadata): %w", err))
		return
	}

	// Still need to re-write the trailer with the new encrypted fileinfo.
	if f.Type == config.FolderTypeReceiveEncrypted {
		err = inWritableDir(func(path string) error {
			fd, err := f.mtimefs.OpenFile(path, fs.OptReadWrite, 0o666)
			if err != nil {
				return err
			}
			defer fd.Close()
			trailerSize, err := writeEncryptionTrailer(file, fd)
			if err != nil {
				return err
			}
			file.EncryptionTrailerSize = int(trailerSize)
			file.Size += trailerSize
			return fd.Truncate(file.Size)
		}, f.mtimefs, file.Name, true)
		if err != nil {
			f.newPullError(file.Name, fmt.Errorf("writing encrypted file trailer: %w", err))
			return
		}
	}

	f.mtimefs.Chtimes(file.Name, file.ModTime(), file.ModTime()) // never fails

	dbUpdateChan <- dbUpdateJob{file, dbUpdateShortcutFile}
}

// copierRoutine reads copierStates until the in channel closes and performs
// the relevant copies when possible, or passes it to the puller routine.
func (f *sendReceiveFolder) copierRoutine(ctx context.Context, in <-chan copyBlocksState, pullChan chan<- pullBlockState, out chan<- *sharedPullerState) {
	otherFolderFilesystems := make(map[string]fs.Filesystem)
	for folder, cfg := range f.model.cfg.Folders() {
		if folder == f.ID {
			continue
		}
		otherFolderFilesystems[folder] = cfg.Filesystem()
	}

	for state := range in {
		if f.Type != config.FolderTypeReceiveEncrypted {
			f.model.progressEmitter.Register(state.sharedPullerState)
		}

		f.setState(FolderSyncing) // Does nothing if already FolderSyncing

	blocks:
		for _, block := range state.blocks {
			select {
			case <-ctx.Done():
				state.fail(fmt.Errorf("folder stopped: %w", ctx.Err()))
				break blocks
			default:
			}

			if !f.DisableSparseFiles && state.reused == 0 && block.IsEmpty() {
				// The block is a block of all zeroes, and we are not reusing
				// a temp file, so there is no need to do anything with it.
				// If we were reusing a temp file and had this block to copy,
				// it would be because the block in the temp file was *not* a
				// block of all zeroes, so then we should not skip it.

				// Pretend we copied it.
				state.skippedSparseBlock(block.Size)
				state.copyDone(block)
				continue
			}

			if f.copyBlock(ctx, block, state, otherFolderFilesystems) {
				state.copyDone(block)
				continue
			}

			if state.failed() != nil {
				break
			}

			state.pullStarted()
			ps := pullBlockState{
				sharedPullerState: state.sharedPullerState,
				block:             block,
			}
			pullChan <- ps
		}
		// If there are no blocks to pull/copy, we still need the temporary file in place.
		if len(state.blocks) == 0 {
			_, err := state.tempFile()
			if err != nil {
				state.fail(err)
			}
		}

		out <- state.sharedPullerState
	}
}

// Returns true when the block was successfully copied.
func (f *sendReceiveFolder) copyBlock(ctx context.Context, block protocol.BlockInfo, state copyBlocksState, otherFolderFilesystems map[string]fs.Filesystem) bool {
	buf := protocol.BufferPool.Get(block.Size)
	defer protocol.BufferPool.Put(buf)

	// Hope that it's usually in the same folder, so start with that
	// one. Also possibly more efficient copy (same filesystem).
	if f.copyBlockFromFolder(ctx, f.ID, block, state, f.mtimefs, buf) {
		return true
	}
	if state.failed() != nil {
		return false
	}

	for folderID, ffs := range otherFolderFilesystems {
		if f.copyBlockFromFolder(ctx, folderID, block, state, ffs, buf) {
			return true
		}
		if state.failed() != nil {
			return false
		}
	}

	return false
}

// Returns true when the block was successfully copied.
// The passed buffer must be large enough to accommodate the block.
func (f *sendReceiveFolder) copyBlockFromFolder(ctx context.Context, folderID string, block protocol.BlockInfo, state copyBlocksState, ffs fs.Filesystem, buf []byte) bool {
	for e, err := range itererr.Zip(f.model.sdb.AllLocalBlocksWithHash(folderID, block.Hash)) {
		if err != nil {
			// We just ignore this and continue pulling instead (though
			// there's a good chance that will fail too, if the DB is
			// unhealthy).
			f.sl.DebugContext(ctx, "Failed to get block information from database", "blockHash", block.Hash, slogutil.FilePath(state.file.Name), slogutil.Error(err))
			return false
		}

		if !f.copyBlockFromFile(ctx, e.FileName, e.Offset, state, ffs, block, buf) {
			if state.failed() != nil {
				return false
			}
			continue
		}

		if e.FileName == state.file.Name {
			state.copiedFromOrigin(block.Size)
		} else {
			state.copiedFromElsewhere(block.Size)
		}
		return true
	}

	return false
}

// Returns true when the block was successfully copied.
// The passed buffer must be large enough to accommodate the block.
func (f *sendReceiveFolder) copyBlockFromFile(ctx context.Context, srcName string, srcOffset int64, state copyBlocksState, ffs fs.Filesystem, block protocol.BlockInfo, buf []byte) bool {
	fd, err := ffs.Open(srcName)
	if err != nil {
		f.sl.DebugContext(ctx, "Failed to open source file for block copy", slogutil.FilePath(srcName), "blockHash", block.Hash, slogutil.Error(err))
		return false
	}
	defer fd.Close()

	_, err = fd.ReadAt(buf, srcOffset)
	if err != nil {
		f.sl.DebugContext(ctx, "Failed to read block from file", slogutil.FilePath(srcName), "blockHash", block.Hash, slogutil.Error(err))
		return false
	}

	// Hash is not SHA256 as it's an encrypted hash token. In that
	// case we can't verify the block integrity so we'll take it on
	// trust. (The other side can and will verify.)
	if f.Type != config.FolderTypeReceiveEncrypted {
		if err := f.verifyBuffer(buf, block); err != nil {
			f.sl.DebugContext(ctx, "Failed to verify block buffer", slogutil.Error(err))
			return false
		}
	}

	dstFd, err := state.tempFile()
	if err != nil {
		// State is already marked as failed when an error is returned here.
		return false
	}

	if f.CopyRangeMethod != config.CopyRangeMethodStandard {
		err = f.withLimiter(ctx, func() error {
			dstFd.mut.Lock()
			defer dstFd.mut.Unlock()
			return fs.CopyRange(f.CopyRangeMethod.ToFS(), fd, dstFd.fd, srcOffset, block.Offset, int64(block.Size))
		})
	} else {
		err = f.limitedWriteAt(ctx, dstFd, buf, block.Offset)
	}
	if err != nil {
		state.fail(fmt.Errorf("dst write: %w", err))
		return false
	}
	return true
}

func (*sendReceiveFolder) verifyBuffer(buf []byte, block protocol.BlockInfo) error {
	if len(buf) != block.Size {
		return fmt.Errorf("length mismatch %d != %d", len(buf), block.Size)
	}

	hash := sha256.Sum256(buf)
	if !bytes.Equal(hash[:], block.Hash) {
		return fmt.Errorf("hash mismatch %x != %x", hash, block.Hash)
	}

	return nil
}

func (f *sendReceiveFolder) pullerRoutine(ctx context.Context, in <-chan pullBlockState, out chan<- *sharedPullerState) {
	requestLimiter := semaphore.New(f.PullerMaxPendingKiB * 1024)
	var wg sync.WaitGroup

	for state := range in {
		if state.failed() != nil {
			out <- state.sharedPullerState
			continue
		}

		f.setState(FolderSyncing) // Does nothing if already FolderSyncing

		// The requestLimiter limits how many pending block requests we have
		// ongoing at any given time, based on the size of the blocks
		// themselves.

		bytes := state.block.Size

		if err := requestLimiter.TakeWithContext(ctx, bytes); err != nil {
			state.fail(err)
			out <- state.sharedPullerState
			continue
		}

		wg.Add(1)

		go func() {
			defer wg.Done()
			defer requestLimiter.Give(bytes)

			f.pullBlock(ctx, state, out)
		}()
	}
	wg.Wait()
}

func (f *sendReceiveFolder) pullBlock(ctx context.Context, state pullBlockState, out chan<- *sharedPullerState) {
	// Get an fd to the temporary file. Technically we don't need it until
	// after fetching the block, but if we run into an error here there is
	// no point in issuing the request to the network.
	fd, err := state.tempFile()
	if err != nil {
		out <- state.sharedPullerState
		return
	}

	if !f.DisableSparseFiles && state.reused == 0 && state.block.IsEmpty() {
		// There is no need to request a block of all zeroes. Pretend we
		// requested it and handled it correctly.
		state.pullDone(state.block)
		out <- state.sharedPullerState
		return
	}

	var lastError error
	candidates := f.model.blockAvailability(f.FolderConfiguration, state.file, state.block)
loop:
	for {
		select {
		case <-ctx.Done():
			state.fail(fmt.Errorf("folder stopped: %w", ctx.Err()))
			break loop
		default:
		}

		// Select the least busy device to pull the block from. If we found no
		// feasible device at all, fail the block (and in the long run, the
		// file).
		found := activity.leastBusy(candidates)
		if found == -1 {
			if lastError != nil {
				state.fail(fmt.Errorf("pull: %w", lastError))
			} else {
				state.fail(fmt.Errorf("pull: %w", errNoDevice))
			}
			break
		}

		selected := candidates[found]
		candidates[found] = candidates[len(candidates)-1]
		candidates = candidates[:len(candidates)-1]

		// Fetch the block, while marking the selected device as in use so that
		// leastBusy can select another device when someone else asks.
		activity.using(selected)
		var buf []byte
		blockNo := int(state.block.Offset / int64(state.file.BlockSize()))
		buf, lastError = f.model.RequestGlobal(ctx, selected.ID, f.folderID, state.file.Name, blockNo, state.block.Offset, state.block.Size, state.block.Hash, selected.FromTemporary)
		activity.done(selected)
		if lastError != nil {
			f.sl.DebugContext(ctx, "Block request returned error", slogutil.FilePath(state.file.Name), "offset", state.block.Offset, "size", state.block.Size, "device", selected.ID.Short(), slogutil.Error(lastError))
			continue
		}

		// Verify that the received block matches the desired hash, if not
		// try pulling it from another device.
		// For receive-only folders, the hash is not SHA256 as it's an
		// encrypted hash token. In that case we can't verify the block
		// integrity so we'll take it on trust. (The other side can and
		// will verify.)
		if f.Type != config.FolderTypeReceiveEncrypted {
			lastError = f.verifyBuffer(buf, state.block)
		}
		if lastError != nil {
			f.sl.DebugContext(ctx, "Block hash mismatch", slogutil.FilePath(state.file.Name), "offset", state.block.Offset, "size", state.block.Size)
			continue
		}

		// Save the block data we got from the cluster
		err = f.limitedWriteAt(ctx, fd, buf, state.block.Offset)
		if err != nil {
			state.fail(fmt.Errorf("save: %w", err))
		} else {
			state.pullDone(state.block)
		}
		break
	}
	out <- state.sharedPullerState
}

func (f *sendReceiveFolder) performFinish(file, curFile protocol.FileInfo, hasCurFile bool, tempName string, dbUpdateChan chan<- dbUpdateJob, scanChan chan<- string) error {
	// Set the correct permission bits on the new file
	if !f.IgnorePerms && !file.NoPermissions {
		if err := f.mtimefs.Chmod(tempName, fs.FileMode(file.Permissions&0o777)); err != nil {
			return fmt.Errorf("setting permissions: %w", err)
		}
	}

	// Set file xattrs and ownership.
	if err := f.setPlatformData(&file, tempName); err != nil {
		return fmt.Errorf("setting metadata: %w", err)
	}

	if stat, err := f.mtimefs.Lstat(file.Name); err == nil {
		// There is an old file or directory already in place. We need to
		// handle that.

		if err := f.scanIfItemChanged(file.Name, stat, curFile, hasCurFile, false, scanChan); err != nil {
			return fmt.Errorf("checking existing file: %w", err)
		}

		if !curFile.IsDirectory() && !curFile.IsSymlink() && file.InConflictWith(curFile) {
			// The new file has been changed in conflict with the existing one. We
			// should file it away as a conflict instead of just removing or
			// archiving.
			// Directories and symlinks aren't checked for conflicts.

			err = f.inWritableDir(func(name string) error {
				return f.moveForConflict(name, file.ModifiedBy.String(), scanChan)
			}, curFile.Name)
		} else {
			err = f.deleteItemOnDisk(curFile, scanChan)
		}
		if err != nil {
			return fmt.Errorf("moving for conflict: %w", err)
		}
	} else if !fs.IsNotExist(err) {
		return fmt.Errorf("checking existing file: %w", err)
	}

	// Replace the original content with the new one. If it didn't work,
	// leave the temp file in place for reuse.
	if err := osutil.RenameOrCopy(f.CopyRangeMethod.ToFS(), f.mtimefs, f.mtimefs, tempName, file.Name); err != nil {
		return fmt.Errorf("replacing file: %w", err)
	}

	// Set the correct timestamp on the new file
	f.mtimefs.Chtimes(file.Name, file.ModTime(), file.ModTime()) // never fails

	// Record the updated file in the index
	dbUpdateChan <- dbUpdateJob{file, dbUpdateHandleFile}
	return nil
}

func (f *sendReceiveFolder) finisherRoutine(ctx context.Context, in <-chan *sharedPullerState, dbUpdateChan chan<- dbUpdateJob, scanChan chan<- string) {
	for state := range in {
		if closed, err := state.finalClose(); closed {
			f.sl.DebugContext(ctx, "Closing temp file", slogutil.FilePath(state.file.Name))

			f.queue.Done(state.file.Name)

			if err == nil {
				err = f.performFinish(state.file, state.curFile, state.hasCurFile, state.tempName, dbUpdateChan, scanChan)
			}

			if err != nil {
				f.newPullError(state.file.Name, fmt.Errorf("finishing: %w", err))
			} else {
				slog.InfoContext(ctx, "Synced file", f.LogAttr(), state.file.LogAttr(), slog.Group("blocks", slog.Int("local", state.reused+state.copyTotal), slog.Int("download", state.pullTotal)))

				minBlocksPerBlock := state.file.BlockSize() / protocol.MinBlockSize
				blockStatsMut.Lock()
				blockStats["total"] += (state.reused + state.copyTotal + state.pullTotal) * minBlocksPerBlock
				blockStats["reused"] += state.reused * minBlocksPerBlock
				blockStats["pulled"] += state.pullTotal * minBlocksPerBlock
				// copyOriginShifted is counted towards copyOrigin due to progress bar reasons
				// for reporting reasons we want to separate these.
				blockStats["copyOrigin"] += state.copyOrigin * minBlocksPerBlock
				blockStats["copyElsewhere"] += (state.copyTotal - state.copyOrigin) * minBlocksPerBlock
				blockStatsMut.Unlock()
			}

			if f.Type != config.FolderTypeReceiveEncrypted {
				f.model.progressEmitter.Deregister(state)
			}

			f.evLogger.Log(events.ItemFinished, map[string]interface{}{
				"folder": f.folderID,
				"item":   state.file.Name,
				"error":  events.Error(err),
				"type":   "file",
				"action": "update",
			})
		}
	}
}

// Moves the given filename to the front of the job queue
func (f *sendReceiveFolder) BringToFront(filename string) {
	f.queue.BringToFront(filename)
}

func (f *sendReceiveFolder) Jobs(page, perpage int) ([]string, []string, int) {
	return f.queue.Jobs(page, perpage)
}

// dbUpdaterRoutine aggregates db updates and commits them in batches no
// larger than 1000 items, and no more delayed than 2 seconds.
func (f *sendReceiveFolder) dbUpdaterRoutine(dbUpdateChan <-chan dbUpdateJob) int {
	const maxBatchTime = 2 * time.Second

	changed := 0
	changedDirs := make(map[string]struct{})
	found := false
	var lastFile protocol.FileInfo
	tick := time.NewTicker(maxBatchTime)
	defer tick.Stop()
	batch := NewFileInfoBatch(func(files []protocol.FileInfo) error {
		// sync directories
		for dir := range changedDirs {
			delete(changedDirs, dir)
			if !f.DisableFsync {
				fd, err := f.mtimefs.Open(dir)
				if err != nil {
					f.sl.Debug("Fsync failed", slogutil.FilePath(dir), slogutil.Error(err))
					continue
				}
				if err := fd.Sync(); err != nil {
					f.sl.Debug("Fsync failed", slogutil.FilePath(dir), slogutil.Error(err))
				}
				fd.Close()
			}
		}

		// All updates to file/folder objects that originated remotely
		// (across the network) use this call to updateLocals
		f.updateLocalsFromPulling(files)

		if found {
			f.ReceivedFile(lastFile.Name, lastFile.IsDeleted())
			found = false
		}

		return nil
	})

loop:
	for {
		select {
		case job, ok := <-dbUpdateChan:
			if !ok {
				break loop
			}

			switch job.jobType {
			case dbUpdateHandleFile, dbUpdateShortcutFile:
				changedDirs[filepath.Dir(job.file.Name)] = struct{}{}
			case dbUpdateHandleDir:
				changedDirs[job.file.Name] = struct{}{}
			case dbUpdateHandleSymlink, dbUpdateInvalidate:
				// fsyncing symlinks is only supported by MacOS
				// and invalidated files are db only changes -> no sync
			}

			// For some reason we seem to care about file deletions and
			// content modification, but not about metadata and dirs/symlinks.
			if !job.file.IsInvalid() && job.jobType&(dbUpdateHandleFile|dbUpdateDeleteFile) != 0 {
				found = true
				lastFile = job.file
			}

			if !job.file.IsDeleted() && !job.file.IsInvalid() {
				// Now that the file is finalized, grab possibly updated
				// inode change time from disk into the local FileInfo. We
				// use this change time to check for changes to xattrs etc
				// on next scan.
				if err := f.updateFileInfoChangeTime(&job.file); err != nil {
					// This means on next scan the likely incorrect change time
					// (resp. whatever caused the error) will cause this file to
					// change. Log at info level to leave a trace if a user
					// notices, but no need to warn
					f.sl.Warn("Failed to update metadata at database commit", slogutil.FilePath(job.file.Name), slogutil.Error(err))
				}
			}
			job.file.Sequence = 0

			batch.Append(job.file)
			batch.FlushIfFull()
			changed++

		case <-tick.C:
			batch.Flush()
		}
	}

	batch.Flush()
	return changed
}

// pullScannerRoutine aggregates paths to be scanned after pulling. The scan is
// scheduled once when scanChan is closed (scanning can not happen during pulling).
func (f *sendReceiveFolder) pullScannerRoutine(ctx context.Context, scanChan <-chan string) {
	toBeScanned := make(map[string]struct{})

	for path := range scanChan {
		toBeScanned[path] = struct{}{}
	}

	if len(toBeScanned) != 0 {
		scanList := make([]string, 0, len(toBeScanned))
		for path := range toBeScanned {
			slog.DebugContext(ctx, "Scheduling scan after pulling", slogutil.FilePath(path))
			scanList = append(scanList, path)
		}
		f.Scan(scanList)
	}
}

func (f *sendReceiveFolder) moveForConflict(name, lastModBy string, scanChan chan<- string) error {
	if isConflict(name) {
		f.sl.Info("Conflict on existing conflict copy; not copying again", slogutil.FilePath(name))
		if err := f.mtimefs.Remove(name); err != nil && !fs.IsNotExist(err) {
			return fmt.Errorf("%s: %w", contextRemovingOldItem, err)
		}
		return nil
	}

	if f.MaxConflicts == 0 {
		if err := f.mtimefs.Remove(name); err != nil && !fs.IsNotExist(err) {
			return fmt.Errorf("%s: %w", contextRemovingOldItem, err)
		}
		return nil
	}

	metricFolderConflictsTotal.WithLabelValues(f.ID).Inc()
	newName := conflictName(name, lastModBy)
	err := f.mtimefs.Rename(name, newName)
	if fs.IsNotExist(err) {
		// We were supposed to move a file away but it does not exist. Either
		// the user has already moved it away, or the conflict was between a
		// remote modification and a local delete. In either way it does not
		// matter, go ahead as if the move succeeded.
		err = nil
	}
	if f.MaxConflicts > -1 {
		matches := existingConflicts(name, f.mtimefs)
		if len(matches) > f.MaxConflicts {
			slices.SortFunc(matches, func(a, b string) int {
				return strings.Compare(b, a)
			})
			for _, match := range matches[f.MaxConflicts:] {
				if gerr := f.mtimefs.Remove(match); gerr != nil {
					f.sl.Debug("Failed to remove extra conflict copy", slogutil.Error(gerr))
				}
			}
		}
	}
	if err == nil {
		scanChan <- newName
	}
	return err
}

func (f *sendReceiveFolder) newPullError(path string, err error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		// Error because the folder stopped - no point logging/tracking
		return
	}

	f.errorsMut.Lock()
	defer f.errorsMut.Unlock()

	// We might get more than one error report for a file (i.e. error on
	// Write() followed by Close()); we keep the first error as that is
	// probably closer to the root cause.
	if _, ok := f.tempPullErrors[path]; ok {
		return
	}

	// Establish context to differentiate from errors while scanning.
	// Use "syncing" as opposed to "pulling" as the latter might be used
	// for errors occurring specifically in the puller routine.
	errStr := fmt.Sprintf("syncing: %s", err)
	f.tempPullErrors[path] = errStr

	f.sl.Debug("New pull error", slogutil.FilePath(path), slogutil.Error(err))
}

// deleteItemOnDisk deletes the file represented by old that is about to be replaced by new.
func (f *sendReceiveFolder) deleteItemOnDisk(item protocol.FileInfo, scanChan chan<- string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%s: %w", contextRemovingOldItem, err)
		}
	}()

	switch {
	case item.IsDirectory():
		// Directories aren't archived and need special treatment due
		// to potential children.
		return f.deleteDirOnDisk(item.Name, scanChan)

	case !item.IsSymlink() && f.versioner != nil:
		// If we should use versioning, let the versioner archive the
		// file before we replace it. Archiving a non-existent file is not
		// an error.
		// Symlinks aren't archived.

		return f.inWritableDir(f.versioner.Archive, item.Name)
	}

	return f.inWritableDir(f.mtimefs.Remove, item.Name)
}

// deleteDirOnDisk attempts to delete a directory. It checks for files/dirs inside
// the directory and removes them if possible or returns an error if it fails
func (f *sendReceiveFolder) deleteDirOnDisk(dir string, scanChan chan<- string) error {
	if err := osutil.TraversesSymlink(f.mtimefs, filepath.Dir(dir)); err != nil {
		return err
	}

	if err := f.deleteDirOnDiskHandleChildren(dir, scanChan); err != nil {
		return err
	}

	err := f.inWritableDir(f.mtimefs.Remove, dir)
	if err == nil || fs.IsNotExist(err) {
		// It was removed or it doesn't exist to start with
		return nil
	}
	if _, serr := f.mtimefs.Lstat(dir); serr != nil && !fs.IsPermission(serr) {
		// We get an error just looking at the directory, and it's not a
		// permission problem. Lets assume the error is in fact some variant
		// of "file does not exist" (possibly expressed as some parent being a
		// file and not a directory etc) and that the delete is handled.
		return nil
	}

	return err
}

func (f *sendReceiveFolder) deleteDirOnDiskHandleChildren(dir string, scanChan chan<- string) error {
	var dirsToDelete []string
	var hasIgnored, hasKnown, hasToBeScanned, hasReceiveOnlyChanged bool
	var delErr error
	err := f.mtimefs.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if path == dir {
			return nil
		}
		if err != nil {
			return err
		}
		switch match := f.ignores.Match(path); {
		case match.IsDeletable():
			if info.IsDir() {
				dirsToDelete = append(dirsToDelete, path)
				return nil
			}
			fallthrough
		case fs.IsTemporary(path):
			if err := f.mtimefs.Remove(path); err != nil && delErr == nil {
				delErr = err
			}
			return nil
		case match.IsIgnored():
			hasIgnored = true
			return nil
		}
		cf, ok, err := f.model.sdb.GetDeviceFile(f.folderID, protocol.LocalDeviceID, path)
		if err != nil {
			return err
		}
		switch {
		case !ok || cf.IsDeleted():
			// Something appeared in the dir that we either are not
			// aware of at all or that we think should be deleted
			// -> schedule scan.
			scanChan <- path
			hasToBeScanned = true
			return nil
		case ok && f.Type == config.FolderTypeReceiveOnly && cf.IsReceiveOnlyChanged():
			hasReceiveOnlyChanged = true
			return nil
		}
		diskFile, err := scanner.CreateFileInfo(info, path, f.mtimefs, f.SyncOwnership, f.SyncXattrs, f.XattrFilter)
		if err != nil {
			// Lets just assume the file has changed.
			scanChan <- path
			hasToBeScanned = true
			return nil //nolint:nilerr
		}
		if !cf.IsEquivalentOptional(diskFile, protocol.FileInfoComparison{
			ModTimeWindow:   f.modTimeWindow,
			IgnorePerms:     f.IgnorePerms,
			IgnoreBlocks:    true,
			IgnoreFlags:     protocol.LocalAllFlags,
			IgnoreOwnership: !f.SyncOwnership,
			IgnoreXattrs:    !f.SyncXattrs,
		}) {
			// File on disk changed compared to what we have in db
			// -> schedule scan.
			scanChan <- path
			hasToBeScanned = true
			return nil
		}
		// Dir contains file that is valid according to db and
		// not ignored -> something weird is going on
		hasKnown = true
		return nil
	})
	if err != nil {
		return err
	}

	for i := range dirsToDelete {
		if err := f.mtimefs.Remove(dirsToDelete[len(dirsToDelete)-1-i]); err != nil && delErr == nil {
			delErr = err
		}
	}

	// "Error precedence":
	// Something changed on disk, check that and maybe all else gets resolved
	if hasToBeScanned {
		return errDirHasToBeScanned
	}
	// Ignored files will never be touched, i.e. this will keep failing until
	// user acts.
	if hasIgnored {
		return errDirHasIgnored
	}
	if hasReceiveOnlyChanged {
		// Pretend we deleted the directory. It will be resurrected as a
		// receive-only changed item on scan.
		scanChan <- dir
		return nil
	}
	if hasKnown {
		return errDirNotEmpty
	}
	// All good, except maybe failing to remove a (?d) ignored item
	return delErr
}

// scanIfItemChanged schedules the given file for scanning and returns errModified
// if it differs from the information in the database. Returns nil if the file has
// not changed.
func (f *sendReceiveFolder) scanIfItemChanged(name string, stat fs.FileInfo, item protocol.FileInfo, hasItem bool, fromDelete bool, scanChan chan<- string) (err error) {
	defer func() {
		if errors.Is(err, errModified) {
			scanChan <- name
		}
	}()

	if !hasItem || item.Deleted {
		// The item appeared from nowhere
		return errModified
	}

	// Check that the item on disk is what we expect it to be according
	// to the database. If there's a mismatch here, there might be local
	// changes that we don't know about yet and we should scan before
	// touching the item.
	statItem, err := scanner.CreateFileInfo(stat, item.Name, f.mtimefs, f.SyncOwnership, f.SyncXattrs, f.XattrFilter)
	if err != nil {
		return fmt.Errorf("comparing item on disk to db: %w", err)
	}
	if !statItem.IsEquivalentOptional(item, protocol.FileInfoComparison{
		ModTimeWindow:   f.modTimeWindow,
		IgnorePerms:     f.IgnorePerms,
		IgnoreBlocks:    true,
		IgnoreFlags:     protocol.LocalAllFlags,
		IgnoreOwnership: fromDelete || !f.SyncOwnership,
		IgnoreXattrs:    fromDelete || !f.SyncXattrs,
	}) {
		return errModified
	}

	return nil
}

// checkToBeDeleted makes sure the file on disk is compatible with what there is
// in the DB before the caller proceeds with actually deleting it.
// I.e. non-nil error status means "Do not delete!" or "is already deleted".
func (f *sendReceiveFolder) checkToBeDeleted(file, cur protocol.FileInfo, hasCur bool, scanChan chan<- string) error {
	if err := osutil.TraversesSymlink(f.mtimefs, filepath.Dir(file.Name)); err != nil {
		f.sl.Debug("Not deleting item behind symlink on disk, but updating database", slogutil.FilePath(file.Name))
		return fs.ErrNotExist
	}

	stat, err := f.mtimefs.Lstat(file.Name)
	deleted := fs.IsNotExist(err) || fs.IsErrCaseConflict(err)
	if !deleted && err != nil {
		return err
	}
	if deleted {
		if hasCur && !cur.Deleted && !cur.IsUnsupported() {
			scanChan <- file.Name
			return errModified
		}
		f.sl.Debug("Not deleting item we don't have, but updating database", slogutil.FilePath(file.Name))
		return err
	}

	return f.scanIfItemChanged(file.Name, stat, cur, hasCur, true, scanChan)
}

// setPlatformData makes adjustments to the metadata that should happen for
// all types (files, directories, symlinks). This should be one of the last
// things we do to a file when syncing changes to it.
func (f *sendReceiveFolder) setPlatformData(file *protocol.FileInfo, name string) error {
	if f.SyncXattrs {
		// Set extended attributes.
		if err := f.mtimefs.SetXattr(name, file.Platform.Xattrs(), f.XattrFilter); errors.Is(err, fs.ErrXattrsNotSupported) {
			f.sl.Debug("Cannot set xattrs (not supported)", slogutil.FilePath(file.Name), slogutil.Error(err))
		} else if err != nil {
			return err
		}
	}

	if f.SyncOwnership {
		// Set ownership based on file metadata.
		if err := f.syncOwnership(file, name); err != nil {
			return err
		}
	} else if f.CopyOwnershipFromParent {
		// Copy the parent owner and group.
		if err := f.copyOwnershipFromParent(name); err != nil {
			return err
		}
	}

	return nil
}

func (f *sendReceiveFolder) copyOwnershipFromParent(path string) error {
	if build.IsWindows {
		// Can't do anything.
		return nil
	}

	info, err := f.mtimefs.Lstat(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("copy owner from parent: %w", err)
	}
	if err := f.mtimefs.Lchown(path, strconv.Itoa(info.Owner()), strconv.Itoa(info.Group())); err != nil {
		return fmt.Errorf("copy owner from parent: %w", err)
	}
	return nil
}

func (f *sendReceiveFolder) inWritableDir(fn func(string) error, path string) error {
	return inWritableDir(fn, f.mtimefs, path, f.IgnorePerms)
}

func (f *sendReceiveFolder) limitedWriteAt(ctx context.Context, fd io.WriterAt, data []byte, offset int64) error {
	return f.withLimiter(ctx, func() error {
		_, err := fd.WriteAt(data, offset)
		return err
	})
}

func (f *sendReceiveFolder) withLimiter(ctx context.Context, fn func() error) error {
	if err := f.writeLimiter.TakeWithContext(ctx, 1); err != nil {
		return err
	}
	defer f.writeLimiter.Give(1)
	return fn()
}

// updateFileInfoChangeTime updates the inode change time in the FileInfo,
// because that depends on the current, new, state of the file on disk.
func (f *sendReceiveFolder) updateFileInfoChangeTime(file *protocol.FileInfo) error {
	info, err := f.mtimefs.Lstat(file.Name)
	if err != nil {
		return err
	}

	if ct := info.InodeChangeTime(); !ct.IsZero() {
		file.InodeChangeNs = ct.UnixNano()
	} else {
		file.InodeChangeNs = 0
	}
	return nil
}

// A []FileError is sent as part of an event and will be JSON serialized.
type FileError struct {
	Path string `json:"path"`
	Err  string `json:"error"`
}

func conflictName(name, lastModBy string) string {
	ext := filepath.Ext(name)
	return name[:len(name)-len(ext)] + time.Now().Format(".sync-conflict-20060102-150405-") + lastModBy + ext
}

func isConflict(name string) bool {
	return strings.Contains(filepath.Base(name), ".sync-conflict-")
}

func existingConflicts(name string, fs fs.Filesystem) []string {
	ext := filepath.Ext(name)
	matches, err := fs.Glob(name[:len(name)-len(ext)] + ".sync-conflict-????????-??????*" + ext)
	if err != nil {
		slog.Debug("Globbing for conflicts failed", slogutil.Error(err))
	}
	return matches
}

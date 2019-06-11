// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"bytes"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/scanner"
	"github.com/syncthing/syncthing/lib/sha256"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/versioner"
	"github.com/syncthing/syncthing/lib/weakhash"
)

var (
	blockStats    = make(map[string]int)
	blockStatsMut = sync.NewMutex()
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
	errDirHasToBeScanned      = errors.New("directory contains unexpected files, scheduling scan")
	errDirHasIgnored          = errors.New("directory contains ignored files (see ignore documentation for (?d) prefix)")
	errDirNotEmpty            = errors.New("directory is not empty; files within are probably ignored on connected devices only")
	errNotAvailable           = errors.New("no connected device has the required version of this file")
	errModified               = errors.New("file modified but not rescanned; will try again later")
	errUnexpectedDirOnFileDel = errors.New("encountered directory when trying to remove file/symlink")
	errIncompatibleSymlink    = errors.New("incompatible symlink entry; rescan with newer Syncthing on source")
	contextRemovingOldItem    = "removing item to be replaced"
)

const (
	dbUpdateHandleDir = iota
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
	jobType int
}

type sendReceiveFolder struct {
	folder

	fs        fs.Filesystem
	versioner versioner.Versioner

	queue *jobQueue

	pullErrors    map[string]string // path -> error string
	pullErrorsMut sync.Mutex
}

func newSendReceiveFolder(model *model, fset *db.FileSet, ignores *ignore.Matcher, cfg config.FolderConfiguration, ver versioner.Versioner, fs fs.Filesystem) service {
	f := &sendReceiveFolder{
		folder:        newFolder(model, fset, ignores, cfg),
		fs:            fs,
		versioner:     ver,
		queue:         newJobQueue(),
		pullErrorsMut: sync.NewMutex(),
	}
	f.folder.puller = f

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
func (f *sendReceiveFolder) pull() bool {
	select {
	case <-f.initialScanFinished:
	default:
		// Once the initial scan finished, a pull will be scheduled
		return true
	}

	// If there is nothing to do, don't even enter pulling state.
	abort := true
	f.fset.WithNeed(protocol.LocalDeviceID, func(intf db.FileIntf) bool {
		abort = false
		return false
	})
	if abort {
		return true
	}

	if err := f.CheckHealth(); err != nil {
		l.Debugln("Skipping pull of", f.Description(), "due to folder error:", err)
		return false
	}

	// Check if the ignore patterns changed.
	oldHash := f.ignores.Hash()
	defer func() {
		if f.ignores.Hash() != oldHash {
			f.ignoresUpdated()
		}
	}()
	if err := f.ignores.Load(".stignore"); err != nil && !fs.IsNotExist(err) {
		err = fmt.Errorf("loading ignores: %v", err)
		f.setError(err)
		return false
	}

	l.Debugf("%v pulling", f)

	f.setState(FolderSyncing)
	f.clearPullErrors()

	scanChan := make(chan string)
	go f.pullScannerRoutine(scanChan)

	defer func() {
		close(scanChan)
		f.setState(FolderIdle)
	}()

	for tries := 0; tries < maxPullerIterations; tries++ {
		select {
		case <-f.ctx.Done():
			return false
		default:
		}

		changed := f.pullerIteration(scanChan)

		l.Debugln(f, "changed", changed, "on try", tries+1)

		if changed == 0 {
			// No files were changed by the puller, so we are in
			// sync. Any errors were just transitional.
			f.clearPullErrors()
			return true
		}
	}

	// We've tried a bunch of times to get in sync, but
	// we're not making it. Probably there are write
	// errors preventing us. Flag this with a warning and
	// wait a bit longer before retrying.
	if errors := f.Errors(); len(errors) > 0 {
		events.Default.Log(events.FolderErrors, map[string]interface{}{
			"folder": f.folderID,
			"errors": errors,
		})
	}

	return false
}

// pullerIteration runs a single puller iteration for the given folder and
// returns the number items that should have been synced (even those that
// might have failed). One puller iteration handles all files currently
// flagged as needed in the folder.
func (f *sendReceiveFolder) pullerIteration(scanChan chan<- string) int {
	pullChan := make(chan pullBlockState)
	copyChan := make(chan copyBlocksState)
	finisherChan := make(chan *sharedPullerState)
	dbUpdateChan := make(chan dbUpdateJob)

	pullWg := sync.NewWaitGroup()
	copyWg := sync.NewWaitGroup()
	doneWg := sync.NewWaitGroup()
	updateWg := sync.NewWaitGroup()

	l.Debugln(f, "copiers:", f.Copiers, "pullerPendingKiB:", f.PullerMaxPendingKiB)

	updateWg.Add(1)
	go func() {
		// dbUpdaterRoutine finishes when dbUpdateChan is closed
		f.dbUpdaterRoutine(dbUpdateChan)
		updateWg.Done()
	}()

	for i := 0; i < f.Copiers; i++ {
		copyWg.Add(1)
		go func() {
			// copierRoutine finishes when copyChan is closed
			f.copierRoutine(copyChan, pullChan, finisherChan)
			copyWg.Done()
		}()
	}

	pullWg.Add(1)
	go func() {
		// pullerRoutine finishes when pullChan is closed
		f.pullerRoutine(pullChan, finisherChan)
		pullWg.Done()
	}()

	doneWg.Add(1)
	// finisherRoutine finishes when finisherChan is closed
	go func() {
		f.finisherRoutine(finisherChan, dbUpdateChan, scanChan)
		doneWg.Done()
	}()

	changed, fileDeletions, dirDeletions, err := f.processNeeded(dbUpdateChan, copyChan, scanChan)

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
		f.processDeletions(fileDeletions, dirDeletions, dbUpdateChan, scanChan)
	}

	// Wait for db updates and scan scheduling to complete
	close(dbUpdateChan)
	updateWg.Wait()

	return changed
}

func (f *sendReceiveFolder) processNeeded(dbUpdateChan chan<- dbUpdateJob, copyChan chan<- copyBlocksState, scanChan chan<- string) (int, map[string]protocol.FileInfo, []protocol.FileInfo, error) {
	defer f.queue.Reset()

	changed := 0
	var dirDeletions []protocol.FileInfo
	fileDeletions := map[string]protocol.FileInfo{}
	buckets := map[string][]protocol.FileInfo{}

	// Iterate the list of items that we need and sort them into piles.
	// Regular files to pull goes into the file queue, everything else
	// (directories, symlinks and deletes) goes into the "process directly"
	// pile.
	f.fset.WithNeed(protocol.LocalDeviceID, func(intf db.FileIntf) bool {
		select {
		case <-f.ctx.Done():
			return false
		default:
		}

		if f.IgnoreDelete && intf.IsDeleted() {
			f.resetPullError(intf.FileName())
			l.Debugln(f, "ignore file deletion (config)", intf.FileName())
			return true
		}

		file := intf.(protocol.FileInfo)

		switch {
		case f.ignores.ShouldIgnore(file.Name):
			f.resetPullError(file.Name)
			file.SetIgnored(f.shortID)
			l.Debugln(f, "Handling ignored file", file)
			dbUpdateChan <- dbUpdateJob{file, dbUpdateInvalidate}
			changed++

		case runtime.GOOS == "windows" && fs.WindowsInvalidFilename(file.Name):
			f.newPullError(file.Name, fs.ErrInvalidFilename)

		case file.IsDeleted():
			if file.IsDirectory() {
				// Perform directory deletions at the end, as we may have
				// files to delete inside them before we get to that point.
				dirDeletions = append(dirDeletions, file)
			} else if file.IsSymlink() {
				f.deleteFile(file, dbUpdateChan, scanChan)
			} else {
				df, ok := f.fset.Get(protocol.LocalDeviceID, file.Name)
				// Local file can be already deleted, but with a lower version
				// number, hence the deletion coming in again as part of
				// WithNeed, furthermore, the file can simply be of the wrong
				// type if we haven't yet managed to pull it.
				if ok && !df.IsDeleted() && !df.IsSymlink() && !df.IsDirectory() && !df.IsInvalid() {
					fileDeletions[file.Name] = file
					// Put files into buckets per first hash
					key := string(df.Blocks[0].Hash)
					buckets[key] = append(buckets[key], df)
				} else {
					f.deleteFileWithCurrent(file, df, ok, dbUpdateChan, scanChan)
				}
			}
			changed++

		case file.Type == protocol.FileInfoTypeFile:
			curFile, hasCurFile := f.fset.Get(protocol.LocalDeviceID, file.Name)
			if _, need := blockDiff(curFile.Blocks, file.Blocks); hasCurFile && len(need) == 0 {
				// We are supposed to copy the entire file, and then fetch nothing. We
				// are only updating metadata, so we don't actually *need* to make the
				// copy.
				f.shortcutFile(file, curFile, dbUpdateChan)
			} else {
				// Queue files for processing after directories and symlinks.
				f.queue.Push(file.Name, file.Size, file.ModTime())
			}

		case runtime.GOOS == "windows" && file.IsSymlink():
			f.resetPullError(file.Name)
			file.SetUnsupported(f.shortID)
			l.Debugln(f, "Invalidating symlink (unsupported)", file.Name)
			dbUpdateChan <- dbUpdateJob{file, dbUpdateInvalidate}
			changed++

		case file.IsDirectory() && !file.IsSymlink():
			changed++
			l.Debugln(f, "Handling directory", file.Name)
			if f.checkParent(file.Name, scanChan) {
				f.handleDir(file, dbUpdateChan, scanChan)
			}

		case file.IsSymlink():
			changed++
			l.Debugln(f, "Handling symlink", file.Name)
			if f.checkParent(file.Name, scanChan) {
				f.handleSymlink(file, dbUpdateChan, scanChan)
			}

		default:
			l.Warnln(file)
			panic("unhandleable item type, can't happen")
		}

		return true
	})

	select {
	case <-f.ctx.Done():
		return changed, nil, nil, f.ctx.Err()
	default:
	}

	// Now do the file queue. Reorder it according to configuration.

	switch f.Order {
	case config.OrderRandom:
		f.queue.Shuffle()
	case config.OrderAlphabetic:
	// The queue is already in alphabetic order.
	case config.OrderSmallestFirst:
		f.queue.SortSmallestFirst()
	case config.OrderLargestFirst:
		f.queue.SortLargestFirst()
	case config.OrderOldestFirst:
		f.queue.SortOldestFirst()
	case config.OrderNewestFirst:
		f.queue.SortNewestFirst()
	}

	// Process the file queue.

nextFile:
	for {
		select {
		case <-f.ctx.Done():
			return changed, fileDeletions, dirDeletions, f.ctx.Err()
		default:
		}

		fileName, ok := f.queue.Pop()
		if !ok {
			break
		}

		f.resetPullError(fileName)

		fi, ok := f.fset.GetGlobal(fileName)
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
		key := string(fi.Blocks[0].Hash)
		for i, candidate := range buckets[key] {
			if protocol.BlocksEqual(candidate.Blocks, fi.Blocks) {
				// Remove the candidate from the bucket
				lidx := len(buckets[key]) - 1
				buckets[key][i] = buckets[key][lidx]
				buckets[key] = buckets[key][:lidx]

				// candidate is our current state of the file, where as the
				// desired state with the delete bit set is in the deletion
				// map.
				desired := fileDeletions[candidate.Name]
				if err := f.renameFile(candidate, desired, fi, dbUpdateChan, scanChan); err != nil {
					// Failed to rename, try to handle files as separate
					// deletions and updates.
					break
				}

				// Remove the pending deletion (as we performed it by renaming)
				delete(fileDeletions, candidate.Name)

				changed++
				f.queue.Done(fileName)
				continue nextFile
			}
		}

		devices := f.fset.Availability(fileName)
		for _, dev := range devices {
			if _, ok := f.model.Connection(dev); ok {
				changed++
				// Handle the file normally, by coping and pulling, etc.
				f.handleFile(fi, copyChan, dbUpdateChan)
				continue nextFile
			}
		}
		f.newPullError(fileName, errNotAvailable)
		f.queue.Done(fileName)
	}

	return changed, fileDeletions, dirDeletions, nil
}

func (f *sendReceiveFolder) processDeletions(fileDeletions map[string]protocol.FileInfo, dirDeletions []protocol.FileInfo, dbUpdateChan chan<- dbUpdateJob, scanChan chan<- string) {
	for _, file := range fileDeletions {
		select {
		case <-f.ctx.Done():
			return
		default:
		}

		f.resetPullError(file.Name)
		f.deleteFile(file, dbUpdateChan, scanChan)
	}

	// Process in reverse order to delete depth first
	for i := range dirDeletions {
		select {
		case <-f.ctx.Done():
			return
		default:
		}

		dir := dirDeletions[len(dirDeletions)-i-1]
		f.resetPullError(dir.Name)
		l.Debugln(f, "Deleting dir", dir.Name)
		f.deleteDir(dir, dbUpdateChan, scanChan)
	}
}

// handleDir creates or updates the given directory
func (f *sendReceiveFolder) handleDir(file protocol.FileInfo, dbUpdateChan chan<- dbUpdateJob, scanChan chan<- string) {
	// Used in the defer closure below, updated by the function body. Take
	// care not declare another err.
	var err error

	f.resetPullError(file.Name)

	events.Default.Log(events.ItemStarted, map[string]string{
		"folder": f.folderID,
		"item":   file.Name,
		"type":   "dir",
		"action": "update",
	})

	defer func() {
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"folder": f.folderID,
			"item":   file.Name,
			"error":  events.Error(err),
			"type":   "dir",
			"action": "update",
		})
	}()

	mode := fs.FileMode(file.Permissions & 0777)
	if f.IgnorePerms || file.NoPermissions {
		mode = 0777
	}

	if shouldDebug() {
		curFile, _ := f.fset.Get(protocol.LocalDeviceID, file.Name)
		l.Debugf("need dir\n\t%v\n\t%v", file, curFile)
	}

	info, err := f.fs.Lstat(file.Name)
	switch {
	// There is already something under that name, we need to handle that.
	// Unless it already is a directory, as we only track permissions,
	// that don't result in a conflict.
	case err == nil && !info.IsDir():
		// Check that it is what we have in the database.
		curFile, hasCurFile := f.model.CurrentFolderFile(f.folderID, file.Name)
		if changed, err := f.itemChanged(info, curFile, hasCurFile, scanChan); err != nil {
			f.newPullError(file.Name, err)
			return
		} else if changed {
			l.Debugln("item changed on disk compared to db; not replacing with dir:", file.Name)
			scanChan <- curFile.Name
			f.newPullError(file.Name, errModified)
			return
		}

		// Remove it to replace with the dir.
		if !curFile.IsSymlink() && f.inConflict(curFile.Version, file.Version) {
			// The new file has been changed in conflict with the existing one. We
			// should file it away as a conflict instead of just removing or
			// archiving. Also merge with the version vector we had, to indicate
			// we have resolved the conflict.
			// Symlinks aren't checked for conflicts.

			file.Version = file.Version.Merge(curFile.Version)
			err = osutil.InWritableDir(func(name string) error {
				return f.moveForConflict(name, file.ModifiedBy.String(), scanChan)
			}, f.fs, curFile.Name)
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
			err = f.fs.Mkdir(path, mode)
			if err != nil || f.IgnorePerms || file.NoPermissions {
				return err
			}

			// Copy the parent owner and group, if we are supposed to do that.
			if err := f.maybeCopyOwner(path); err != nil {
				return err
			}

			// Stat the directory so we can check its permissions.
			info, err := f.fs.Lstat(path)
			if err != nil {
				return err
			}

			// Mask for the bits we want to preserve and add them in to the
			// directories permissions.
			return f.fs.Chmod(path, mode|(info.Mode()&retainBits))
		}

		if err = osutil.InWritableDir(mkdir, f.fs, file.Name); err == nil {
			dbUpdateChan <- dbUpdateJob{file, dbUpdateHandleDir}
		} else {
			f.newPullError(file.Name, errors.Wrap(err, "creating directory"))
		}
		return
	// Weird error when stat()'ing the dir. Probably won't work to do
	// anything else with it if we can't even stat() it.
	case err != nil:
		f.newPullError(file.Name, errors.Wrap(err, "checking file to be replaced"))
		return
	}

	// The directory already exists, so we just correct the mode bits. (We
	// don't handle modification times on directories, because that sucks...)
	// It's OK to change mode bits on stuff within non-writable directories.
	if !f.IgnorePerms && !file.NoPermissions {
		if err := f.fs.Chmod(file.Name, mode|(fs.FileMode(info.Mode())&retainBits)); err != nil {
			f.newPullError(file.Name, err)
			return
		}
	}
	dbUpdateChan <- dbUpdateJob{file, dbUpdateHandleDir}
}

// checkParent verifies that the thing we are handling lives inside a directory,
// and not a symlink or regular file. It also resurrects missing parent dirs.
func (f *sendReceiveFolder) checkParent(file string, scanChan chan<- string) bool {
	parent := filepath.Dir(file)

	if err := osutil.TraversesSymlink(f.fs, parent); err != nil {
		f.newPullError(file, errors.Wrap(err, "checking parent dirs"))
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
	if _, err := f.fs.Lstat(parent); !fs.IsNotExist(err) {
		l.Debugf("%v parent not missing %v", f, file)
		return true
	}
	l.Debugf("%v resurrecting parent directory of %v", f, file)
	if err := f.fs.MkdirAll(parent, 0755); err != nil {
		f.newPullError(file, errors.Wrap(err, "resurrecting parent dir"))
		return false
	}
	scanChan <- parent
	return true
}

// handleSymlink creates or updates the given symlink
func (f *sendReceiveFolder) handleSymlink(file protocol.FileInfo, dbUpdateChan chan<- dbUpdateJob, scanChan chan<- string) {
	// Used in the defer closure below, updated by the function body. Take
	// care not declare another err.
	var err error

	f.resetPullError(file.Name)

	events.Default.Log(events.ItemStarted, map[string]string{
		"folder": f.folderID,
		"item":   file.Name,
		"type":   "symlink",
		"action": "update",
	})

	defer func() {
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"folder": f.folderID,
			"item":   file.Name,
			"error":  events.Error(err),
			"type":   "symlink",
			"action": "update",
		})
	}()

	if shouldDebug() {
		curFile, _ := f.fset.Get(protocol.LocalDeviceID, file.Name)
		l.Debugf("need symlink\n\t%v\n\t%v", file, curFile)
	}

	if len(file.SymlinkTarget) == 0 {
		// Index entry from a Syncthing predating the support for including
		// the link target in the index entry. We log this as an error.
		f.newPullError(file.Name, errIncompatibleSymlink)
		return
	}

	// There is already something under that name, we need to handle that.
	if info, err := f.fs.Lstat(file.Name); err == nil {
		// Check that it is what we have in the database.
		curFile, hasCurFile := f.model.CurrentFolderFile(f.folderID, file.Name)
		if changed, err := f.itemChanged(info, curFile, hasCurFile, scanChan); err != nil {
			f.newPullError(file.Name, err)
			return
		} else if changed {
			l.Debugln("item changed on disk compared to db; not replacing with symlink:", file.Name)
			scanChan <- curFile.Name
			f.newPullError(file.Name, errModified)
			return
		}
		// Remove it to replace with the symlink. This also handles the
		// "change symlink type" path.
		if !curFile.IsDirectory() && !curFile.IsSymlink() && f.inConflict(curFile.Version, file.Version) {
			// The new file has been changed in conflict with the existing one. We
			// should file it away as a conflict instead of just removing or
			// archiving. Also merge with the version vector we had, to indicate
			// we have resolved the conflict.
			// Directories and symlinks aren't checked for conflicts.

			file.Version = file.Version.Merge(curFile.Version)
			err = osutil.InWritableDir(func(name string) error {
				return f.moveForConflict(name, file.ModifiedBy.String(), scanChan)
			}, f.fs, curFile.Name)
		} else {
			err = f.deleteItemOnDisk(curFile, scanChan)
		}
		if err != nil {
			f.newPullError(file.Name, errors.Wrap(err, "symlink remove"))
			return
		}
	}

	// We declare a function that acts on only the path name, so
	// we can pass it to InWritableDir.
	createLink := func(path string) error {
		if err := f.fs.CreateSymlink(file.SymlinkTarget, path); err != nil {
			return err
		}
		return f.maybeCopyOwner(path)
	}

	if err = osutil.InWritableDir(createLink, f.fs, file.Name); err == nil {
		dbUpdateChan <- dbUpdateJob{file, dbUpdateHandleSymlink}
	} else {
		f.newPullError(file.Name, errors.Wrap(err, "symlink create"))
	}
}

// deleteDir attempts to remove a directory that was deleted on a remote
func (f *sendReceiveFolder) deleteDir(file protocol.FileInfo, dbUpdateChan chan<- dbUpdateJob, scanChan chan<- string) {
	// Used in the defer closure below, updated by the function body. Take
	// care not declare another err.
	var err error

	events.Default.Log(events.ItemStarted, map[string]string{
		"folder": f.folderID,
		"item":   file.Name,
		"type":   "dir",
		"action": "delete",
	})

	defer func() {
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"folder": f.folderID,
			"item":   file.Name,
			"error":  events.Error(err),
			"type":   "dir",
			"action": "delete",
		})
	}()

	if err = f.deleteDirOnDisk(file.Name, scanChan); err != nil {
		f.newPullError(file.Name, errors.Wrap(err, "delete dir"))
		return
	}

	dbUpdateChan <- dbUpdateJob{file, dbUpdateDeleteDir}
}

// deleteFile attempts to delete the given file
func (f *sendReceiveFolder) deleteFile(file protocol.FileInfo, dbUpdateChan chan<- dbUpdateJob, scanChan chan<- string) {
	cur, hasCur := f.fset.Get(protocol.LocalDeviceID, file.Name)
	f.deleteFileWithCurrent(file, cur, hasCur, dbUpdateChan, scanChan)
}

func (f *sendReceiveFolder) deleteFileWithCurrent(file, cur protocol.FileInfo, hasCur bool, dbUpdateChan chan<- dbUpdateJob, scanChan chan<- string) {
	// Used in the defer closure below, updated by the function body. Take
	// care not declare another err.
	var err error

	l.Debugln(f, "Deleting file", file.Name)

	f.resetPullError(file.Name)

	events.Default.Log(events.ItemStarted, map[string]string{
		"folder": f.folderID,
		"item":   file.Name,
		"type":   "file",
		"action": "delete",
	})

	defer func() {
		if err != nil {
			f.newPullError(file.Name, errors.Wrap(err, "delete file"))
		}
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"folder": f.folderID,
			"item":   file.Name,
			"error":  events.Error(err),
			"type":   "file",
			"action": "delete",
		})
	}()

	if !hasCur {
		// We should never try to pull a deletion for a file we don't have in the DB.
		l.Debugln(f, "not deleting file we don't have", file.Name)
		dbUpdateChan <- dbUpdateJob{file, dbUpdateDeleteFile}
		return
	}
	if err = f.checkToBeDeleted(cur, scanChan); err != nil {
		return
	}

	// We are asked to delete a file, but what we have on disk and in db
	// is a directory. Something is wrong here, should probably not happen.
	if cur.IsDirectory() {
		err = errUnexpectedDirOnFileDel
		return
	}

	if f.inConflict(cur.Version, file.Version) {
		// There is a conflict here, which shouldn't happen as deletions
		// always lose. Merge the version vector of the file we have
		// locally and commit it to db to resolve the conflict.
		cur.Version = cur.Version.Merge(file.Version)
		dbUpdateChan <- dbUpdateJob{cur, dbUpdateHandleFile}
		return
	}

	if f.versioner != nil && !cur.IsSymlink() {
		err = osutil.InWritableDir(f.versioner.Archive, f.fs, file.Name)
	} else {
		err = osutil.InWritableDir(f.fs.Remove, f.fs, file.Name)
	}

	if err == nil || fs.IsNotExist(err) {
		// It was removed or it doesn't exist to start with
		dbUpdateChan <- dbUpdateJob{file, dbUpdateDeleteFile}
		return
	}

	if _, serr := f.fs.Lstat(file.Name); serr != nil && !fs.IsPermission(serr) {
		// We get an error just looking at the file, and it's not a permission
		// problem. Lets assume the error is in fact some variant of "file
		// does not exist" (possibly expressed as some parent being a file and
		// not a directory etc) and that the delete is handled.
		err = nil
		dbUpdateChan <- dbUpdateJob{file, dbUpdateDeleteFile}
	}
}

// renameFile attempts to rename an existing file to a destination
// and set the right attributes on it.
func (f *sendReceiveFolder) renameFile(cur, source, target protocol.FileInfo, dbUpdateChan chan<- dbUpdateJob, scanChan chan<- string) error {
	// Used in the defer closure below, updated by the function body. Take
	// care not declare another err.
	var err error

	events.Default.Log(events.ItemStarted, map[string]string{
		"folder": f.folderID,
		"item":   source.Name,
		"type":   "file",
		"action": "delete",
	})
	events.Default.Log(events.ItemStarted, map[string]string{
		"folder": f.folderID,
		"item":   target.Name,
		"type":   "file",
		"action": "update",
	})

	defer func() {
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"folder": f.folderID,
			"item":   source.Name,
			"error":  events.Error(err),
			"type":   "file",
			"action": "delete",
		})
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"folder": f.folderID,
			"item":   target.Name,
			"error":  events.Error(err),
			"type":   "file",
			"action": "update",
		})
	}()

	l.Debugln(f, "taking rename shortcut", source.Name, "->", target.Name)

	// Check that source is compatible with what we have in the DB
	if err = f.checkToBeDeleted(cur, scanChan); err != nil {
		return err
	}
	// Check that the target corresponds to what we have in the DB
	curTarget, ok := f.fset.Get(protocol.LocalDeviceID, target.Name)
	switch stat, serr := f.fs.Lstat(target.Name); {
	case serr != nil && fs.IsNotExist(serr):
		if !ok || curTarget.IsDeleted() {
			break
		}
		scanChan <- target.Name
		err = errModified
	case serr != nil:
		// We can't check whether the file changed as compared to the db,
		// do not delete.
		err = serr
	case !ok:
		// Target appeared from nowhere
		scanChan <- target.Name
		err = errModified
	default:
		var fi protocol.FileInfo
		if fi, err = scanner.CreateFileInfo(stat, target.Name, f.fs); err == nil {
			if !fi.IsEquivalentOptional(curTarget, f.IgnorePerms, true, protocol.LocalAllFlags) {
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
		err = f.CheckAvailableSpace(source.Size)
		if err == nil {
			err = osutil.Copy(f.fs, f.fs, source.Name, tempName)
			if err == nil {
				err = osutil.InWritableDir(f.versioner.Archive, f.fs, source.Name)
			}
		}
	} else {
		err = osutil.RenameOrCopy(f.fs, f.fs, source.Name, tempName)
	}
	if err != nil {
		return err
	}

	blockStatsMut.Lock()
	blockStats["total"] += len(target.Blocks)
	blockStats["renamed"] += len(target.Blocks)
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
func (f *sendReceiveFolder) handleFile(file protocol.FileInfo, copyChan chan<- copyBlocksState, dbUpdateChan chan<- dbUpdateJob) {
	curFile, hasCurFile := f.fset.Get(protocol.LocalDeviceID, file.Name)

	have, _ := blockDiff(curFile.Blocks, file.Blocks)

	tempName := fs.TempName(file.Name)

	populateOffsets(file.Blocks)

	blocks := make([]protocol.BlockInfo, 0, len(file.Blocks))
	var blocksSize int64
	reused := make([]int32, 0, len(file.Blocks))

	// Check for an old temporary file which might have some blocks we could
	// reuse.
	tempBlocks, err := scanner.HashFile(f.ctx, f.fs, tempName, file.BlockSize(), nil, false)
	if err == nil {
		// Check for any reusable blocks in the temp file
		tempCopyBlocks, _ := blockDiff(tempBlocks, file.Blocks)

		// block.String() returns a string unique to the block
		existingBlocks := make(map[string]struct{}, len(tempCopyBlocks))
		for _, block := range tempCopyBlocks {
			existingBlocks[block.String()] = struct{}{}
		}

		// Since the blocks are already there, we don't need to get them.
		for i, block := range file.Blocks {
			_, ok := existingBlocks[block.String()]
			if !ok {
				blocks = append(blocks, block)
				blocksSize += int64(block.Size)
			} else {
				reused = append(reused, int32(i))
			}
		}

		// The sharedpullerstate will know which flags to use when opening the
		// temp file depending if we are reusing any blocks or not.
		if len(reused) == 0 {
			// Otherwise, discard the file ourselves in order for the
			// sharedpuller not to panic when it fails to exclusively create a
			// file which already exists
			osutil.InWritableDir(f.fs.Remove, f.fs, tempName)
		}
	} else {
		// Copy the blocks, as we don't want to shuffle them on the FileInfo
		blocks = append(blocks, file.Blocks...)
		blocksSize = file.Size
	}

	if err := f.CheckAvailableSpace(blocksSize); err != nil {
		f.newPullError(file.Name, err)
		f.queue.Done(file.Name)
		return
	}

	// Shuffle the blocks
	rand.Shuffle(blocks)

	events.Default.Log(events.ItemStarted, map[string]string{
		"folder": f.folderID,
		"item":   file.Name,
		"type":   "file",
		"action": "update",
	})

	s := sharedPullerState{
		file:             file,
		fs:               f.fs,
		folder:           f.folderID,
		tempName:         tempName,
		realName:         file.Name,
		copyTotal:        len(blocks),
		copyNeeded:       len(blocks),
		reused:           len(reused),
		updated:          time.Now(),
		available:        reused,
		availableUpdated: time.Now(),
		ignorePerms:      f.IgnorePerms || file.NoPermissions,
		hasCurFile:       hasCurFile,
		curFile:          curFile,
		mut:              sync.NewRWMutex(),
		sparse:           !f.DisableSparseFiles,
		created:          time.Now(),
	}

	l.Debugf("%v need file %s; copy %d, reused %v", f, file.Name, len(blocks), len(reused))

	cs := copyBlocksState{
		sharedPullerState: &s,
		blocks:            blocks,
		have:              len(have),
	}
	copyChan <- cs
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

// shortcutFile sets file mode and modification time, when that's the only
// thing that has changed.
func (f *sendReceiveFolder) shortcutFile(file, curFile protocol.FileInfo, dbUpdateChan chan<- dbUpdateJob) {
	l.Debugln(f, "taking shortcut on", file.Name)

	f.resetPullError(file.Name)

	events.Default.Log(events.ItemStarted, map[string]string{
		"folder": f.folderID,
		"item":   file.Name,
		"type":   "file",
		"action": "metadata",
	})

	var err error
	defer events.Default.Log(events.ItemFinished, map[string]interface{}{
		"folder": f.folderID,
		"item":   file.Name,
		"error":  events.Error(err),
		"type":   "file",
		"action": "metadata",
	})

	f.queue.Done(file.Name)

	if !f.IgnorePerms && !file.NoPermissions {
		if err = f.fs.Chmod(file.Name, fs.FileMode(file.Permissions&0777)); err != nil {
			f.newPullError(file.Name, err)
			return
		}
	}

	f.fs.Chtimes(file.Name, file.ModTime(), file.ModTime()) // never fails

	// This may have been a conflict. We should merge the version vectors so
	// that our clock doesn't move backwards.
	file.Version = file.Version.Merge(curFile.Version)

	dbUpdateChan <- dbUpdateJob{file, dbUpdateShortcutFile}
}

// copierRoutine reads copierStates until the in channel closes and performs
// the relevant copies when possible, or passes it to the puller routine.
func (f *sendReceiveFolder) copierRoutine(in <-chan copyBlocksState, pullChan chan<- pullBlockState, out chan<- *sharedPullerState) {
	buf := protocol.BufferPool.Get(protocol.MinBlockSize)
	defer func() {
		protocol.BufferPool.Put(buf)
	}()

	for state := range in {
		dstFd, err := state.tempFile()
		if err != nil {
			// Nothing more to do for this failed file, since we couldn't create a temporary for it.
			out <- state.sharedPullerState
			continue
		}

		f.model.progressEmitter.Register(state.sharedPullerState)

		folderFilesystems := make(map[string]fs.Filesystem)
		var folders []string
		for folder, cfg := range f.model.cfg.Folders() {
			folderFilesystems[folder] = cfg.Filesystem()
			folders = append(folders, folder)
		}

		var file fs.File
		var weakHashFinder *weakhash.Finder

		blocksPercentChanged := 0
		if tot := len(state.file.Blocks); tot > 0 {
			blocksPercentChanged = (tot - state.have) * 100 / tot
		}

		if blocksPercentChanged >= f.WeakHashThresholdPct {
			hashesToFind := make([]uint32, 0, len(state.blocks))
			for _, block := range state.blocks {
				if block.WeakHash != 0 {
					hashesToFind = append(hashesToFind, block.WeakHash)
				}
			}

			if len(hashesToFind) > 0 {
				file, err = f.fs.Open(state.file.Name)
				if err == nil {
					weakHashFinder, err = weakhash.NewFinder(f.ctx, file, int(state.file.BlockSize()), hashesToFind)
					if err != nil {
						l.Debugln("weak hasher", err)
					}
				}
			} else {
				l.Debugf("not weak hashing %s. file did not contain any weak hashes", state.file.Name)
			}
		} else {
			l.Debugf("not weak hashing %s. not enough changed %.02f < %d", state.file.Name, blocksPercentChanged, f.WeakHashThresholdPct)
		}

	blocks:
		for _, block := range state.blocks {
			select {
			case <-f.ctx.Done():
				state.fail(errors.Wrap(f.ctx.Err(), "folder stopped"))
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
				state.copiedFromOrigin()
				state.copyDone(block)
				continue
			}

			buf = protocol.BufferPool.Upgrade(buf, int(block.Size))

			found, err := weakHashFinder.Iterate(block.WeakHash, buf, func(offset int64) bool {
				if verifyBuffer(buf, block) != nil {
					return true
				}

				_, err = dstFd.WriteAt(buf, block.Offset)
				if err != nil {
					state.fail(errors.Wrap(err, "dst write"))

				}
				if offset == block.Offset {
					state.copiedFromOrigin()
				} else {
					state.copiedFromOriginShifted()
				}

				return false
			})
			if err != nil {
				l.Debugln("weak hasher iter", err)
			}

			if !found {
				found = f.model.finder.Iterate(folders, block.Hash, func(folder, path string, index int32) bool {
					fs := folderFilesystems[folder]
					fd, err := fs.Open(path)
					if err != nil {
						return false
					}

					_, err = fd.ReadAt(buf, int64(state.file.BlockSize())*int64(index))
					fd.Close()
					if err != nil {
						return false
					}

					if err := verifyBuffer(buf, block); err != nil {
						l.Debugln("Finder failed to verify buffer", err)
						return false
					}

					_, err = dstFd.WriteAt(buf, block.Offset)
					if err != nil {
						state.fail(errors.Wrap(err, "dst write"))
					}
					if path == state.file.Name {
						state.copiedFromOrigin()
					}
					return true
				})
			}

			if state.failed() != nil {
				break
			}

			if !found {
				state.pullStarted()
				ps := pullBlockState{
					sharedPullerState: state.sharedPullerState,
					block:             block,
				}
				pullChan <- ps
			} else {
				state.copyDone(block)
			}
		}
		if file != nil {
			// os.File used to return invalid argument if nil.
			// fs.File panics as it's an interface.
			file.Close()
		}

		out <- state.sharedPullerState
	}
}

func verifyBuffer(buf []byte, block protocol.BlockInfo) error {
	if len(buf) != int(block.Size) {
		return fmt.Errorf("length mismatch %d != %d", len(buf), block.Size)
	}
	hf := sha256.New()
	_, err := hf.Write(buf)
	if err != nil {
		return err
	}
	hash := hf.Sum(nil)

	if !bytes.Equal(hash, block.Hash) {
		return fmt.Errorf("hash mismatch %x != %x", hash, block.Hash)
	}

	return nil
}

func (f *sendReceiveFolder) pullerRoutine(in <-chan pullBlockState, out chan<- *sharedPullerState) {
	requestLimiter := newByteSemaphore(f.PullerMaxPendingKiB * 1024)
	wg := sync.NewWaitGroup()

	for state := range in {
		if state.failed() != nil {
			out <- state.sharedPullerState
			continue
		}

		// The requestLimiter limits how many pending block requests we have
		// ongoing at any given time, based on the size of the blocks
		// themselves.

		state := state
		bytes := int(state.block.Size)

		requestLimiter.take(bytes)
		wg.Add(1)

		go func() {
			defer wg.Done()
			defer requestLimiter.give(bytes)

			f.pullBlock(state, out)
		}()
	}
	wg.Wait()
}

func (f *sendReceiveFolder) pullBlock(state pullBlockState, out chan<- *sharedPullerState) {
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
	candidates := f.model.Availability(f.folderID, state.file, state.block)
	for {
		select {
		case <-f.ctx.Done():
			state.fail(errors.Wrap(f.ctx.Err(), "folder stopped"))
			return
		default:
		}

		// Select the least busy device to pull the block from. If we found no
		// feasible device at all, fail the block (and in the long run, the
		// file).
		selected, found := activity.leastBusy(candidates)
		if !found {
			if lastError != nil {
				state.fail(errors.Wrap(lastError, "pull"))
			} else {
				state.fail(errors.Wrap(errNoDevice, "pull"))
			}
			break
		}

		candidates = removeAvailability(candidates, selected)

		// Fetch the block, while marking the selected device as in use so that
		// leastBusy can select another device when someone else asks.
		activity.using(selected)
		var buf []byte
		buf, lastError = f.model.requestGlobal(selected.ID, f.folderID, state.file.Name, state.block.Offset, int(state.block.Size), state.block.Hash, state.block.WeakHash, selected.FromTemporary)
		activity.done(selected)
		if lastError != nil {
			l.Debugln("request:", f.folderID, state.file.Name, state.block.Offset, state.block.Size, "returned error:", lastError)
			continue
		}

		// Verify that the received block matches the desired hash, if not
		// try pulling it from another device.
		lastError = verifyBuffer(buf, state.block)
		if lastError != nil {
			l.Debugln("request:", f.folderID, state.file.Name, state.block.Offset, state.block.Size, "hash mismatch")
			continue
		}

		// Save the block data we got from the cluster
		_, err = fd.WriteAt(buf, state.block.Offset)
		if err != nil {
			state.fail(errors.Wrap(err, "save"))
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
		if err := f.fs.Chmod(tempName, fs.FileMode(file.Permissions&0777)); err != nil {
			return err
		}
	}

	// Copy the parent owner and group, if we are supposed to do that.
	if err := f.maybeCopyOwner(tempName); err != nil {
		return err
	}

	if stat, err := f.fs.Lstat(file.Name); err == nil {
		// There is an old file or directory already in place. We need to
		// handle that.

		if changed, err := f.itemChanged(stat, curFile, hasCurFile, scanChan); err != nil {
			return err
		} else if changed {
			l.Debugln("file changed on disk compared to db; not finishing:", file.Name)
			scanChan <- curFile.Name
			return errModified
		}

		if !curFile.IsDirectory() && !curFile.IsSymlink() && f.inConflict(curFile.Version, file.Version) {
			// The new file has been changed in conflict with the existing one. We
			// should file it away as a conflict instead of just removing or
			// archiving. Also merge with the version vector we had, to indicate
			// we have resolved the conflict.
			// Directories and symlinks aren't checked for conflicts.

			file.Version = file.Version.Merge(curFile.Version)
			err = osutil.InWritableDir(func(name string) error {
				return f.moveForConflict(name, file.ModifiedBy.String(), scanChan)
			}, f.fs, curFile.Name)
		} else {
			err = f.deleteItemOnDisk(curFile, scanChan)
		}
		if err != nil {
			return err
		}
	}

	// Replace the original content with the new one. If it didn't work,
	// leave the temp file in place for reuse.
	if err := osutil.RenameOrCopy(f.fs, f.fs, tempName, file.Name); err != nil {
		return err
	}

	// Set the correct timestamp on the new file
	f.fs.Chtimes(file.Name, file.ModTime(), file.ModTime()) // never fails

	// Record the updated file in the index
	dbUpdateChan <- dbUpdateJob{file, dbUpdateHandleFile}
	return nil
}

func (f *sendReceiveFolder) finisherRoutine(in <-chan *sharedPullerState, dbUpdateChan chan<- dbUpdateJob, scanChan chan<- string) {
	for state := range in {
		if closed, err := state.finalClose(); closed {
			l.Debugln(f, "closing", state.file.Name)

			f.queue.Done(state.file.Name)

			if err == nil {
				err = f.performFinish(state.file, state.curFile, state.hasCurFile, state.tempName, dbUpdateChan, scanChan)
			}

			if err != nil {
				f.newPullError(state.file.Name, err)
			} else {
				blockStatsMut.Lock()
				blockStats["total"] += state.reused + state.copyTotal + state.pullTotal
				blockStats["reused"] += state.reused
				blockStats["pulled"] += state.pullTotal
				// copyOriginShifted is counted towards copyOrigin due to progress bar reasons
				// for reporting reasons we want to separate these.
				blockStats["copyOrigin"] += state.copyOrigin - state.copyOriginShifted
				blockStats["copyOriginShifted"] += state.copyOriginShifted
				blockStats["copyElsewhere"] += state.copyTotal - state.copyOrigin
				blockStatsMut.Unlock()
			}

			f.model.progressEmitter.Deregister(state)

			events.Default.Log(events.ItemFinished, map[string]interface{}{
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

func (f *sendReceiveFolder) Jobs() ([]string, []string) {
	return f.queue.Jobs()
}

// dbUpdaterRoutine aggregates db updates and commits them in batches no
// larger than 1000 items, and no more delayed than 2 seconds.
func (f *sendReceiveFolder) dbUpdaterRoutine(dbUpdateChan <-chan dbUpdateJob) {
	const maxBatchTime = 2 * time.Second

	batch := newFileInfoBatch(nil)
	tick := time.NewTicker(maxBatchTime)
	defer tick.Stop()

	changedDirs := make(map[string]struct{})
	found := false
	var lastFile protocol.FileInfo

	batch.flushFn = func(files []protocol.FileInfo) error {
		// sync directories
		for dir := range changedDirs {
			delete(changedDirs, dir)
			fd, err := f.fs.Open(dir)
			if err != nil {
				l.Debugf("fsync %q failed: %v", dir, err)
				continue
			}
			if err := fd.Sync(); err != nil {
				l.Debugf("fsync %q failed: %v", dir, err)
			}
			fd.Close()
		}

		// All updates to file/folder objects that originated remotely
		// (across the network) use this call to updateLocals
		f.updateLocalsFromPulling(files)

		if found {
			f.ReceivedFile(lastFile.Name, lastFile.IsDeleted())
			found = false
		}

		return nil
	}

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

			job.file.Sequence = 0

			batch.append(job.file)

			batch.flushIfFull()

		case <-tick.C:
			batch.flush()
		}
	}

	batch.flush()
}

// pullScannerRoutine aggregates paths to be scanned after pulling. The scan is
// scheduled once when scanChan is closed (scanning can not happen during pulling).
func (f *sendReceiveFolder) pullScannerRoutine(scanChan <-chan string) {
	toBeScanned := make(map[string]struct{})

	for path := range scanChan {
		toBeScanned[path] = struct{}{}
	}

	if len(toBeScanned) != 0 {
		scanList := make([]string, 0, len(toBeScanned))
		for path := range toBeScanned {
			l.Debugln(f, "scheduling scan after pulling for", path)
			scanList = append(scanList, path)
		}
		f.Scan(scanList)
	}
}

func (f *sendReceiveFolder) inConflict(current, replacement protocol.Vector) bool {
	if current.Concurrent(replacement) {
		// Obvious case
		return true
	}
	if replacement.Counter(f.shortID) > current.Counter(f.shortID) {
		// The replacement file contains a higher version for ourselves than
		// what we have. This isn't supposed to be possible, since it's only
		// we who can increment that counter. We take it as a sign that
		// something is wrong (our index may have been corrupted or removed)
		// and flag it as a conflict.
		return true
	}
	return false
}

func removeAvailability(availabilities []Availability, availability Availability) []Availability {
	for i := range availabilities {
		if availabilities[i] == availability {
			availabilities[i] = availabilities[len(availabilities)-1]
			return availabilities[:len(availabilities)-1]
		}
	}
	return availabilities
}

func (f *sendReceiveFolder) moveForConflict(name, lastModBy string, scanChan chan<- string) error {
	if isConflict(name) {
		l.Infoln("Conflict for", name, "which is already a conflict copy; not copying again.")
		if err := f.fs.Remove(name); err != nil && !fs.IsNotExist(err) {
			return errors.Wrap(err, contextRemovingOldItem)
		}
		return nil
	}

	if f.MaxConflicts == 0 {
		if err := f.fs.Remove(name); err != nil && !fs.IsNotExist(err) {
			return errors.Wrap(err, contextRemovingOldItem)
		}
		return nil
	}

	newName := conflictName(name, lastModBy)
	err := f.fs.Rename(name, newName)
	if fs.IsNotExist(err) {
		// We were supposed to move a file away but it does not exist. Either
		// the user has already moved it away, or the conflict was between a
		// remote modification and a local delete. In either way it does not
		// matter, go ahead as if the move succeeded.
		err = nil
	}
	if f.MaxConflicts > -1 {
		matches := existingConflicts(name, f.fs)
		if len(matches) > f.MaxConflicts {
			sort.Sort(sort.Reverse(sort.StringSlice(matches)))
			for _, match := range matches[f.MaxConflicts:] {
				if gerr := f.fs.Remove(match); gerr != nil {
					l.Debugln(f, "removing extra conflict", gerr)
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
	f.pullErrorsMut.Lock()
	defer f.pullErrorsMut.Unlock()

	// We might get more than one error report for a file (i.e. error on
	// Write() followed by Close()); we keep the first error as that is
	// probably closer to the root cause.
	if _, ok := f.pullErrors[path]; ok {
		return
	}

	l.Infof("Puller (folder %s, item %q): %v", f.Description(), path, err)

	// Establish context to differentiate from errors while scanning.
	// Use "syncing" as opposed to "pulling" as the latter might be used
	// for errors occurring specificly in the puller routine.
	f.pullErrors[path] = fmt.Sprintln("syncing:", err)
}

// resetPullError removes the error at path in case there was an error on a
// previous pull iteration.
func (f *sendReceiveFolder) resetPullError(path string) {
	f.pullErrorsMut.Lock()
	delete(f.pullErrors, path)
	f.pullErrorsMut.Unlock()
}

func (f *sendReceiveFolder) clearPullErrors() {
	f.pullErrorsMut.Lock()
	f.pullErrors = make(map[string]string)
	f.pullErrorsMut.Unlock()
}

func (f *sendReceiveFolder) Errors() []FileError {
	scanErrors := f.folder.Errors()
	f.pullErrorsMut.Lock()
	errors := make([]FileError, 0, len(f.pullErrors)+len(f.scanErrors))
	for path, err := range f.pullErrors {
		errors = append(errors, FileError{path, err})
	}
	f.pullErrorsMut.Unlock()
	errors = append(errors, scanErrors...)
	sort.Sort(fileErrorList(errors))
	return errors
}

// deleteItemOnDisk deletes the file represented by old that is about to be replaced by new.
func (f *sendReceiveFolder) deleteItemOnDisk(item protocol.FileInfo, scanChan chan<- string) (err error) {
	defer func() {
		err = errors.Wrap(err, contextRemovingOldItem)
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

		return osutil.InWritableDir(f.versioner.Archive, f.fs, item.Name)
	}

	return osutil.InWritableDir(f.fs.Remove, f.fs, item.Name)
}

// deleteDirOnDisk attempts to delete a directory. It checks for files/dirs inside
// the directory and removes them if possible or returns an error if it fails
func (f *sendReceiveFolder) deleteDirOnDisk(dir string, scanChan chan<- string) error {
	files, _ := f.fs.DirNames(dir)

	toBeDeleted := make([]string, 0, len(files))

	hasIgnored := false
	hasKnown := false
	hasToBeScanned := false

	for _, dirFile := range files {
		fullDirFile := filepath.Join(dir, dirFile)
		if fs.IsTemporary(dirFile) || f.ignores.Match(fullDirFile).IsDeletable() {
			toBeDeleted = append(toBeDeleted, fullDirFile)
		} else if f.ignores != nil && f.ignores.Match(fullDirFile).IsIgnored() {
			hasIgnored = true
		} else if cf, ok := f.fset.Get(protocol.LocalDeviceID, fullDirFile); !ok || cf.IsDeleted() || cf.IsInvalid() {
			// Something appeared in the dir that we either are not aware of
			// at all, that we think should be deleted or that is invalid,
			// but not currently ignored -> schedule scan. The scanChan
			// might be nil, in which case we trust the scanning to be
			// handled later as a result of our error return.
			if scanChan != nil {
				scanChan <- fullDirFile
			}
			hasToBeScanned = true
		} else {
			// Dir contains file that is valid according to db and
			// not ignored -> something weird is going on
			hasKnown = true
		}
	}

	if hasToBeScanned {
		return errDirHasToBeScanned
	}
	if hasIgnored {
		return errDirHasIgnored
	}
	if hasKnown {
		return errDirNotEmpty
	}

	for _, del := range toBeDeleted {
		f.fs.RemoveAll(del)
	}

	err := osutil.InWritableDir(f.fs.Remove, f.fs, dir)
	if err == nil || fs.IsNotExist(err) {
		// It was removed or it doesn't exist to start with
		return nil
	}
	if _, serr := f.fs.Lstat(dir); serr != nil && !fs.IsPermission(serr) {
		// We get an error just looking at the directory, and it's not a
		// permission problem. Lets assume the error is in fact some variant
		// of "file does not exist" (possibly expressed as some parent being a
		// file and not a directory etc) and that the delete is handled.
		return nil
	}

	return err
}

// itemChanged returns true if the given disk file differs from the information
// in the database and schedules that file for scanning
func (f *sendReceiveFolder) itemChanged(stat fs.FileInfo, item protocol.FileInfo, hasItem bool, scanChan chan<- string) (changed bool, err error) {
	defer func() {
		if changed {
			scanChan <- item.Name
		}
	}()

	if !hasItem || item.Deleted {
		// The item appeared from nowhere
		return true, nil
	}

	// Check that the item on disk is what we expect it to be according
	// to the database. If there's a mismatch here, there might be local
	// changes that we don't know about yet and we should scan before
	// touching the item.
	statItem, err := scanner.CreateFileInfo(stat, item.Name, f.fs)
	if err != nil {
		return false, errors.Wrap(err, "comparing item on disk to db")
	}

	return !statItem.IsEquivalentOptional(item, f.IgnorePerms, true, protocol.LocalAllFlags), nil
}

// checkToBeDeleted makes sure the file on disk is compatible with what there is
// in the DB before the caller proceeds with actually deleting it.
// I.e. non-nil error status means "Do not delete!".
func (f *sendReceiveFolder) checkToBeDeleted(cur protocol.FileInfo, scanChan chan<- string) error {
	stat, err := f.fs.Lstat(cur.Name)
	if err != nil {
		if fs.IsNotExist(err) {
			// File doesn't exist to start with.
			return nil
		}
		// We can't check whether the file changed as compared to the db,
		// do not delete.
		return err
	}
	changed, err := f.itemChanged(stat, cur, true, scanChan)
	if err != nil {
		return err
	}
	if changed {
		return errModified
	}
	return nil
}

func (f *sendReceiveFolder) maybeCopyOwner(path string) error {
	if !f.CopyOwnershipFromParent {
		// Not supposed to do anything.
		return nil
	}
	if runtime.GOOS == "windows" {
		// Can't do anything.
		return nil
	}

	info, err := f.fs.Lstat(filepath.Dir(path))
	if err != nil {
		return errors.Wrap(err, "copy owner from parent")
	}
	if err := f.fs.Lchown(path, info.Owner(), info.Group()); err != nil {
		return errors.Wrap(err, "copy owner from parent")
	}
	return nil
}

// A []FileError is sent as part of an event and will be JSON serialized.
type FileError struct {
	Path string `json:"path"`
	Err  string `json:"error"`
}

type fileErrorList []FileError

func (l fileErrorList) Len() int {
	return len(l)
}

func (l fileErrorList) Less(a, b int) bool {
	return l[a].Path < l[b].Path
}

func (l fileErrorList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
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
		l.Debugln("globbing for conflicts", err)
	}
	return matches
}

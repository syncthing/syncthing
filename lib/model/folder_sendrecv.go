// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	stdsync "sync"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
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
	activity               = newDeviceActivity()
	errNoDevice            = errors.New("peers who had this file went away, or the file has changed while syncing. will retry later")
	errSymlinksUnsupported = errors.New("symlinks not supported")
	errDirHasToBeScanned   = errors.New("directory contains unexpected files, scheduling scan")
	errDirHasIgnored       = errors.New("directory contains ignored files (see ignore documentation for (?d) prefix)")
	errDirNotEmpty         = errors.New("directory is not empty")
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
	defaultCopiers              = 2
	defaultPullerPause          = 60 * time.Second
	defaultPullerPendingKiB     = 2 * protocol.MaxBlockSize / 1024
	defaultPullerMaxQueueLength = 1000

	maxPullerIterations = 3
)

type dbUpdateJob struct {
	file    protocol.FileInfo
	jobType int
}

type sendReceiveFolder struct {
	folder

	prevIgnoreHash string
	fs             fs.Filesystem
	versioner      versioner.Versioner

	queue *jobQueue

	errors    map[string]string // path -> error string
	errorsMut sync.Mutex
}

func newSendReceiveFolder(model *Model, cfg config.FolderConfiguration, ver versioner.Versioner, fs fs.Filesystem) service {
	f := &sendReceiveFolder{
		folder:    newFolder(model, cfg),
		fs:        fs,
		versioner: ver,
		queue:     newJobQueue(),
		errorsMut: sync.NewMutex(),
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

func (f *sendReceiveFolder) pull() bool {
	select {
	case <-f.initialScanFinished:
	default:
		// Once the initial scan finished, a pull will be scheduled
		return true
	}

	if err := f.CheckHealth(); err != nil {
		l.Debugln("Skipping pull of", f.Description(), "due to folder error:", err)
		return true
	}

	f.model.fmut.RLock()
	curIgnores := f.model.folderIgnores[f.folderID]
	f.model.fmut.RUnlock()

	curIgnoreHash := curIgnores.Hash()
	ignoresChanged := curIgnoreHash != f.prevIgnoreHash

	l.Debugf("%v pulling (ignoresChanged=%v)", f, ignoresChanged)

	f.setState(FolderSyncing)
	f.clearErrors()

	scanChan := make(chan string)
	go f.pullScannerRoutine(scanChan)

	success := true
	tries := 0

	for {
		changed, finished := f.pullerIteration(curIgnores, ignoresChanged, scanChan)
		if finished {
			tries++
		}

		l.Debugln(f, "try", tries, "changed", changed, "finished", finished)

		if changed == 0 {
			// No files were changed by the puller, so we are in
			// sync.
			break
		}

		if tries == maxPullerIterations {
			// We've tried a bunch of times to get in sync, but
			// we're not making it. Probably there are write
			// errors preventing us. Flag this with a warning and
			// wait a bit longer before retrying.
			success = false
			if folderErrors := f.PullErrors(); len(folderErrors) > 0 {
				events.Default.Log(events.FolderErrors, map[string]interface{}{
					"folder": f.folderID,
					"errors": folderErrors,
				})
			}
			break
		}
	}

	f.setState(FolderIdle)

	close(scanChan)

	if success {
		f.prevIgnoreHash = curIgnoreHash
		return true
	}

	return false
}

// pullerIteration runs a single puller iteration for the given folder and
// returns the number items that should have been synced (even those that
// might have failed). One puller iteration handles all files currently
// flagged as needed in the folder.
func (f *sendReceiveFolder) pullerIteration(ignores *ignore.Matcher, ignoresChanged bool, scanChan chan<- string) (changed int, finished bool) {
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
		f.finisherRoutine(ignores, finisherChan, dbUpdateChan, scanChan)
		doneWg.Done()
	}()

	f.model.fmut.RLock()
	folderFiles := f.model.folderFiles[f.folderID]
	f.model.fmut.RUnlock()

	maxQueueLen := f.PullerMaxQueueLength
	if maxQueueLen <= 0 {
		maxQueueLen = defaultPullerMaxQueueLength
	}

	changed = 0
	finished = true // we assume by default that we are going to finish
	var processDirectly []protocol.FileInfo

	// Iterate the list of items that we need and sort them into piles.
	// Regular files to pull goes into the file queue, everything else
	// (directories, symlinks and deletes) goes into the "process directly"
	// pile.
	folderFiles.WithNeed(protocol.LocalDeviceID, func(intf db.FileIntf) bool {
		if f.IgnoreDelete && intf.IsDeleted() {
			l.Debugln(f, "ignore file deletion (config)", intf.FileName())
			return true
		}

		file := intf.(protocol.FileInfo)

		switch {
		case ignores.ShouldIgnore(file.Name):
			file.Invalidate(f.shortID)
			l.Debugln(f, "Handling ignored file", file)
			dbUpdateChan <- dbUpdateJob{file, dbUpdateInvalidate}

		case runtime.GOOS == "windows" && fs.WindowsInvalidFilename(file.Name):
			f.newError("need", file.Name, fs.ErrInvalidFilename)
			changed++
			return true

		case file.IsDeleted():
			processDirectly = append(processDirectly, file)
			changed++

		case file.Type == protocol.FileInfoTypeFile:
			// Queue files for processing after directories and symlinks, if
			// it has availability.

			devices := folderFiles.Availability(file.Name)
			for _, dev := range devices {
				if _, ok := f.model.Connection(dev); ok {
					f.queue.Push(file.Name, file.Size, file.ModTime())
					changed++

					if changed >= maxQueueLen {
						// We're done for now; set the finished flag to
						// false so the caller knows we did not do
						// everything we could, and stop iterating.
						finished = false
						return false
					}

					return true
				}
			}
			l.Debugln(f, "Needed file is unavailable", file)

		case runtime.GOOS == "windows" && file.IsSymlink():
			file.Invalidate(f.shortID)
			l.Debugln(f, "Invalidating symlink (unsupported)", file.Name)
			dbUpdateChan <- dbUpdateJob{file, dbUpdateInvalidate}

		default:
			// Directories, symlinks
			processDirectly = append(processDirectly, file)
			changed++
		}

		return true
	})

	// Sort the "process directly" pile by number of path components. This
	// ensures that we handle parents before children.

	sort.Sort(byComponentCount(processDirectly))

	// Process the list.

	fileDeletions := map[string]protocol.FileInfo{}
	dirDeletions := []protocol.FileInfo{}
	buckets := map[string][]protocol.FileInfo{}

	for _, fi := range processDirectly {
		// Verify that the thing we are handling lives inside a directory,
		// and not a symlink or empty space.
		if err := osutil.TraversesSymlink(f.fs, filepath.Dir(fi.Name)); err != nil {
			f.newError("traverses d", fi.Name, err)
			continue
		}

		switch {
		case fi.IsDeleted():
			// A deleted file, directory or symlink
			if fi.IsDirectory() {
				// Perform directory deletions at the end, as we may have
				// files to delete inside them before we get to that point.
				dirDeletions = append(dirDeletions, fi)
			} else {
				fileDeletions[fi.Name] = fi
				df, ok := f.model.CurrentFolderFile(f.folderID, fi.Name)
				// Local file can be already deleted, but with a lower version
				// number, hence the deletion coming in again as part of
				// WithNeed, furthermore, the file can simply be of the wrong
				// type if we haven't yet managed to pull it.
				if ok && !df.IsDeleted() && !df.IsSymlink() && !df.IsDirectory() && !df.IsInvalid() {
					// Put files into buckets per first hash
					key := string(df.Blocks[0].Hash)
					buckets[key] = append(buckets[key], df)
				}
			}

		case fi.IsDirectory() && !fi.IsSymlink():
			l.Debugln(f, "Handling directory", fi.Name)
			f.handleDir(fi, dbUpdateChan)

		case fi.IsSymlink():
			l.Debugln("Handling symlink", fi.Name)
			l.Debugln(f, "Handling symlink", fi.Name)
			f.handleSymlink(fi, dbUpdateChan)

		default:
			l.Warnln(fi)
			panic("unhandleable item type, can't happen")
		}
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
			// Stop processing files if the puller has been told to stop.
			break nextFile
		default:
		}

		fileName, ok := f.queue.Pop()
		if !ok {
			break
		}

		fi, ok := f.model.CurrentGlobalFile(f.folderID, fileName)
		if !ok {
			// File is no longer in the index. Mark it as done and drop it.
			f.queue.Done(fileName)
			continue
		}

		if fi.IsDeleted() || fi.Type != protocol.FileInfoTypeFile {
			// The item has changed type or status in the index while we
			// were processing directories above.
			f.queue.Done(fileName)
			continue
		}

		dirName := filepath.Dir(fi.Name)

		// Verify that the thing we are handling lives inside a directory,
		// and not a symlink or empty space.
		if err := osutil.TraversesSymlink(f.fs, dirName); err != nil {
			f.newError("traverses q", fi.Name, err)
			continue
		}

		// issues #114 and #4475: This works around a race condition
		// between two devices, when one device removes a directory and the
		// other creates a file in it. However that happens, we end up with
		// a directory for "foo" with the delete bit, but a file "foo/bar"
		// that we want to sync. We never create the directory, and hence
		// fail to create the file and end up looping forever on it. This
		// breaks that by creating the directory and scheduling a scan,
		// where it will be found and the delete bit on it removed.  The
		// user can then clean up as they like...
		if _, err := f.fs.Lstat(dirName); fs.IsNotExist(err) {
			l.Debugln("%v resurrecting parent directory of %v", f, fi.Name)
			if err := f.fs.MkdirAll(dirName, 0755); err != nil {
				f.newError("resurrecting parent dir", fi.Name, err)
				continue
			}
			scanChan <- dirName
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
				// Remove the pending deletion (as we perform it by renaming)
				delete(fileDeletions, candidate.Name)

				f.renameFile(desired, fi, dbUpdateChan)

				f.queue.Done(fileName)
				continue nextFile
			}
		}

		// Handle the file normally, by coping and pulling, etc.
		f.handleFile(fi, copyChan, finisherChan, dbUpdateChan)
	}

	// Signal copy and puller routines that we are done with the in data for
	// this iteration. Wait for them to finish.
	close(copyChan)
	copyWg.Wait()
	close(pullChan)
	pullWg.Wait()

	// Signal the finisher chan that there will be no more input.
	close(finisherChan)

	// Wait for the finisherChan to finish.
	doneWg.Wait()

	for _, file := range fileDeletions {
		l.Debugln(f, "Deleting file", file.Name)
		f.deleteFile(file, dbUpdateChan)
	}

	for i := range dirDeletions {
		dir := dirDeletions[len(dirDeletions)-i-1]
		l.Debugln(f, "Deleting dir", dir.Name)
		f.handleDeleteDir(dir, ignores, dbUpdateChan, scanChan)
	}

	// Wait for db updates and scan scheduling to complete
	close(dbUpdateChan)
	updateWg.Wait()

	return changed, finished
}

// handleDir creates or updates the given directory
func (f *sendReceiveFolder) handleDir(file protocol.FileInfo, dbUpdateChan chan<- dbUpdateJob) {
	// Used in the defer closure below, updated by the function body. Take
	// care not declare another err.
	var err error

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
		curFile, _ := f.model.CurrentFolderFile(f.folderID, file.Name)
		l.Debugf("need dir\n\t%v\n\t%v", file, curFile)
	}

	info, err := f.fs.Lstat(file.Name)
	switch {
	// There is already something under that name, but it's a file/link.
	// Most likely a file/link is getting replaced with a directory.
	// Remove the file/link and fall through to directory creation.
	case err == nil && (!info.IsDir() || info.IsSymlink()):
		err = osutil.InWritableDir(f.fs.Remove, f.fs, file.Name)
		if err != nil {
			f.newError("dir replace", file.Name, err)
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
			f.newError("dir mkdir", file.Name, err)
		}
		return
	// Weird error when stat()'ing the dir. Probably won't work to do
	// anything else with it if we can't even stat() it.
	case err != nil:
		f.newError("dir stat", file.Name, err)
		return
	}

	// The directory already exists, so we just correct the mode bits. (We
	// don't handle modification times on directories, because that sucks...)
	// It's OK to change mode bits on stuff within non-writable directories.
	if f.IgnorePerms || file.NoPermissions {
		dbUpdateChan <- dbUpdateJob{file, dbUpdateHandleDir}
	} else if err := f.fs.Chmod(file.Name, mode|(fs.FileMode(info.Mode())&retainBits)); err == nil {
		dbUpdateChan <- dbUpdateJob{file, dbUpdateHandleDir}
	} else {
		f.newError("dir chmod", file.Name, err)
	}
}

// handleSymlink creates or updates the given symlink
func (f *sendReceiveFolder) handleSymlink(file protocol.FileInfo, dbUpdateChan chan<- dbUpdateJob) {
	// Used in the defer closure below, updated by the function body. Take
	// care not declare another err.
	var err error

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
		curFile, _ := f.model.CurrentFolderFile(f.folderID, file.Name)
		l.Debugf("need symlink\n\t%v\n\t%v", file, curFile)
	}

	if len(file.SymlinkTarget) == 0 {
		// Index entry from a Syncthing predating the support for including
		// the link target in the index entry. We log this as an error.
		err = errors.New("incompatible symlink entry; rescan with newer Syncthing on source")
		f.newError("symlink", file.Name, err)
		return
	}

	if _, err = f.fs.Lstat(file.Name); err == nil {
		// There is already something under that name. Remove it to replace
		// with the symlink. This also handles the "change symlink type"
		// path.
		err = osutil.InWritableDir(f.fs.Remove, f.fs, file.Name)
		if err != nil {
			f.newError("symlink remove", file.Name, err)
			return
		}
	}

	// We declare a function that acts on only the path name, so
	// we can pass it to InWritableDir.
	createLink := func(path string) error {
		return f.fs.CreateSymlink(file.SymlinkTarget, path)
	}

	if err = osutil.InWritableDir(createLink, f.fs, file.Name); err == nil {
		dbUpdateChan <- dbUpdateJob{file, dbUpdateHandleSymlink}
	} else {
		f.newError("symlink create", file.Name, err)
	}
}

// handleDeleteDir attempts to remove a directory that was deleted on a remote
func (f *sendReceiveFolder) handleDeleteDir(file protocol.FileInfo, ignores *ignore.Matcher, dbUpdateChan chan<- dbUpdateJob, scanChan chan<- string) {
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

	err = f.deleteDir(file.Name, ignores, scanChan)

	if err != nil {
		f.newError("delete dir", file.Name, err)
		return
	}

	dbUpdateChan <- dbUpdateJob{file, dbUpdateDeleteDir}
}

// deleteFile attempts to delete the given file
func (f *sendReceiveFolder) deleteFile(file protocol.FileInfo, dbUpdateChan chan<- dbUpdateJob) {
	// Used in the defer closure below, updated by the function body. Take
	// care not declare another err.
	var err error

	events.Default.Log(events.ItemStarted, map[string]string{
		"folder": f.folderID,
		"item":   file.Name,
		"type":   "file",
		"action": "delete",
	})

	defer func() {
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"folder": f.folderID,
			"item":   file.Name,
			"error":  events.Error(err),
			"type":   "file",
			"action": "delete",
		})
	}()

	cur, ok := f.model.CurrentFolderFile(f.folderID, file.Name)
	if ok && f.inConflict(cur.Version, file.Version) {
		// There is a conflict here. Move the file to a conflict copy instead
		// of deleting. Also merge with the version vector we had, to indicate
		// we have resolved the conflict.
		file.Version = file.Version.Merge(cur.Version)
		err = osutil.InWritableDir(func(name string) error {
			return f.moveForConflict(name, file.ModifiedBy.String())
		}, f.fs, file.Name)
	} else if f.versioner != nil && !cur.IsSymlink() {
		err = osutil.InWritableDir(f.versioner.Archive, f.fs, file.Name)
	} else {
		err = osutil.InWritableDir(f.fs.Remove, f.fs, file.Name)
	}

	if err == nil || fs.IsNotExist(err) {
		// It was removed or it doesn't exist to start with
		dbUpdateChan <- dbUpdateJob{file, dbUpdateDeleteFile}
	} else if _, serr := f.fs.Lstat(file.Name); serr != nil && !fs.IsPermission(serr) {
		// We get an error just looking at the file, and it's not a permission
		// problem. Lets assume the error is in fact some variant of "file
		// does not exist" (possibly expressed as some parent being a file and
		// not a directory etc) and that the delete is handled.
		dbUpdateChan <- dbUpdateJob{file, dbUpdateDeleteFile}
	} else {
		f.newError("delete file", file.Name, err)
	}
}

// renameFile attempts to rename an existing file to a destination
// and set the right attributes on it.
func (f *sendReceiveFolder) renameFile(source, target protocol.FileInfo, dbUpdateChan chan<- dbUpdateJob) {
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

	if f.versioner != nil {
		err = osutil.Copy(f.fs, source.Name, target.Name)
		if err == nil {
			err = osutil.InWritableDir(f.versioner.Archive, f.fs, source.Name)
		}
	} else {
		err = osutil.TryRename(f.fs, source.Name, target.Name)
	}

	if err == nil {
		blockStatsMut.Lock()
		blockStats["total"] += len(target.Blocks)
		blockStats["renamed"] += len(target.Blocks)
		blockStatsMut.Unlock()

		// The file was renamed, so we have handled both the necessary delete
		// of the source and the creation of the target. Fix-up the metadata,
		// and update the local index of the target file.

		dbUpdateChan <- dbUpdateJob{source, dbUpdateDeleteFile}

		err = f.shortcutFile(target)
		if err != nil {
			err = fmt.Errorf("from %s: %s", source.Name, err.Error())
			f.newError("rename shortcut", target.Name, err)
			return
		}

		dbUpdateChan <- dbUpdateJob{target, dbUpdateHandleFile}
	} else {
		// We failed the rename so we have a source file that we still need to
		// get rid of. Attempt to delete it instead so that we make *some*
		// progress. The target is unhandled.

		err = osutil.InWritableDir(f.fs.Remove, f.fs, source.Name)
		if err != nil {
			err = fmt.Errorf("from %s: %s", source.Name, err.Error())
			f.newError("rename delete", target.Name, err)
			return
		}

		dbUpdateChan <- dbUpdateJob{source, dbUpdateDeleteFile}
	}
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
func (f *sendReceiveFolder) handleFile(file protocol.FileInfo, copyChan chan<- copyBlocksState, finisherChan chan<- *sharedPullerState, dbUpdateChan chan<- dbUpdateJob) {
	curFile, hasCurFile := f.model.CurrentFolderFile(f.folderID, file.Name)

	have, need := blockDiff(curFile.Blocks, file.Blocks)

	if hasCurFile && len(need) == 0 {
		// We are supposed to copy the entire file, and then fetch nothing. We
		// are only updating metadata, so we don't actually *need* to make the
		// copy.
		l.Debugln(f, "taking shortcut on", file.Name)

		events.Default.Log(events.ItemStarted, map[string]string{
			"folder": f.folderID,
			"item":   file.Name,
			"type":   "file",
			"action": "metadata",
		})

		f.queue.Done(file.Name)

		err := f.shortcutFile(file)
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"folder": f.folderID,
			"item":   file.Name,
			"error":  events.Error(err),
			"type":   "file",
			"action": "metadata",
		})

		if err != nil {
			f.newError("shortcut", file.Name, err)
		} else {
			dbUpdateChan <- dbUpdateJob{file, dbUpdateShortcutFile}
		}

		return
	}

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

	if f.MinDiskFree.BaseValue() > 0 {
		if usage, err := f.fs.Usage("."); err == nil && usage.Free < blocksSize {
			l.Warnf(`Folder "%s": insufficient disk space in %s for %s: have %.2f MiB, need %.2f MiB`, f.folderID, f.fs.URI(), file.Name, float64(usage.Free)/1024/1024, float64(blocksSize)/1024/1024)
			f.newError("disk space", file.Name, errors.New("insufficient space"))
			return
		}
	}

	// Shuffle the blocks
	for i := range blocks {
		j := rand.Intn(i + 1)
		blocks[i], blocks[j] = blocks[j], blocks[i]
	}

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
func (f *sendReceiveFolder) shortcutFile(file protocol.FileInfo) error {
	if !f.IgnorePerms && !file.NoPermissions {
		if err := f.fs.Chmod(file.Name, fs.FileMode(file.Permissions&0777)); err != nil {
			f.newError("shortcut chmod", file.Name, err)
			return err
		}
	}

	f.fs.Chtimes(file.Name, file.ModTime(), file.ModTime()) // never fails

	// This may have been a conflict. We should merge the version vectors so
	// that our clock doesn't move backwards.
	if cur, ok := f.model.CurrentFolderFile(f.folderID, file.Name); ok {
		file.Version = file.Version.Merge(cur.Version)
	}

	return nil
}

// copierRoutine reads copierStates until the in channel closes and performs
// the relevant copies when possible, or passes it to the puller routine.
func (f *sendReceiveFolder) copierRoutine(in <-chan copyBlocksState, pullChan chan<- pullBlockState, out chan<- *sharedPullerState) {
	buf := make([]byte, protocol.MinBlockSize)

	for state := range in {
		dstFd, err := state.tempFile()
		if err != nil {
			// Nothing more to do for this failed file, since we couldn't create a temporary for it.
			out <- state.sharedPullerState
			continue
		}

		if f.model.progressEmitter != nil {
			f.model.progressEmitter.Register(state.sharedPullerState)
		}

		folderFilesystems := make(map[string]fs.Filesystem)
		var folders []string
		f.model.fmut.RLock()
		for folder, cfg := range f.model.folderCfgs {
			folderFilesystems[folder] = cfg.Filesystem()
			folders = append(folders, folder)
		}
		f.model.fmut.RUnlock()

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
					weakHashFinder, err = weakhash.NewFinder(file, int(state.file.BlockSize()), hashesToFind)
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

		for _, block := range state.blocks {
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

			if s := int(block.Size); s > cap(buf) {
				buf = make([]byte, s)
			} else {
				buf = buf[:s]
			}

			found, err := weakHashFinder.Iterate(block.WeakHash, buf, func(offset int64) bool {
				if verifyBuffer(buf, block) != nil {
					return true
				}

				_, err = dstFd.WriteAt(buf, block.Offset)
				if err != nil {
					state.fail("dst write", err)

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
						state.fail("dst write", err)
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
		// Select the least busy device to pull the block from. If we found no
		// feasible device at all, fail the block (and in the long run, the
		// file).
		selected, found := activity.leastBusy(candidates)
		if !found {
			if lastError != nil {
				state.fail("pull", lastError)
			} else {
				state.fail("pull", errNoDevice)
			}
			break
		}

		candidates = removeAvailability(candidates, selected)

		// Fetch the block, while marking the selected device as in use so that
		// leastBusy can select another device when someone else asks.
		activity.using(selected)
		buf, lastError := f.model.requestGlobal(selected.ID, f.folderID, state.file.Name, state.block.Offset, int(state.block.Size), state.block.Hash, state.block.WeakHash, selected.FromTemporary)
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
			state.fail("save", err)
		} else {
			state.pullDone(state.block)
		}
		break
	}
	out <- state.sharedPullerState
}

func (f *sendReceiveFolder) performFinish(ignores *ignore.Matcher, state *sharedPullerState, dbUpdateChan chan<- dbUpdateJob, scanChan chan<- string) error {
	// Set the correct permission bits on the new file
	if !f.IgnorePerms && !state.file.NoPermissions {
		if err := f.fs.Chmod(state.tempName, fs.FileMode(state.file.Permissions&0777)); err != nil {
			return err
		}
	}

	if stat, err := f.fs.Lstat(state.file.Name); err == nil {
		// There is an old file or directory already in place. We need to
		// handle that.

		curMode := uint32(stat.Mode())
		if runtime.GOOS == "windows" && osutil.IsWindowsExecutable(state.file.Name) {
			curMode |= 0111
		}

		// Check that the file on disk is what we expect it to be according
		// to the database. If there's a mismatch here, there might be local
		// changes that we don't know about yet and we should scan before
		// touching the file. There is also a case where we think the file
		// should be there, but it was removed, which is a conflict, yet
		// creations always wins when competing with a deletion, so no need
		// to handle that specially.
		changed := false
		switch {
		case !state.hasCurFile:
			// The file appeared from nowhere
			l.Debugln("file exists but not scanned; not finishing:", state.file.Name)
			changed = true

		case stat.IsDir() != state.curFile.IsDirectory() || stat.IsSymlink() != state.curFile.IsSymlink():
			// The file changed type. IsRegular is implicitly tested in the condition above
			l.Debugln("file type changed but not rescanned; not finishing:", state.curFile.Name)
			changed = true

		case stat.IsRegular():
			if !stat.ModTime().Equal(state.curFile.ModTime()) || stat.Size() != state.curFile.Size {
				l.Debugln("file modified but not rescanned; not finishing:", state.curFile.Name)
				changed = true
				break
			}
			// check permissions
			fallthrough

		case stat.IsDir():
			// Dirs only have perm, no modetime/size
			if !f.IgnorePerms && !state.curFile.NoPermissions && state.curFile.HasPermissionBits() && !protocol.PermsEqual(state.curFile.Permissions, curMode) {
				l.Debugln("file permission modified but not rescanned; not finishing:", state.curFile.Name)
				changed = true
			}
		}

		if changed {
			scanChan <- state.curFile.Name
			return fmt.Errorf("file modified but not rescanned; will try again later")
		}

		switch {
		case stat.IsDir() || stat.IsSymlink():
			// It's a directory or a symlink. These are not versioned or
			// archived for conflicts, only removed (which of course fails for
			// non-empty directories).

			if err = f.deleteDir(state.file.Name, ignores, scanChan); err != nil {
				return err
			}

		case f.inConflict(state.curFile.Version, state.file.Version):
			// The new file has been changed in conflict with the existing one. We
			// should file it away as a conflict instead of just removing or
			// archiving. Also merge with the version vector we had, to indicate
			// we have resolved the conflict.

			state.file.Version = state.file.Version.Merge(state.curFile.Version)
			err = osutil.InWritableDir(func(name string) error {
				return f.moveForConflict(name, state.file.ModifiedBy.String())
			}, f.fs, state.file.Name)
			if err != nil {
				return err
			}

		case f.versioner != nil && !state.file.IsSymlink():
			// If we should use versioning, let the versioner archive the old
			// file before we replace it. Archiving a non-existent file is not
			// an error.

			if err = osutil.InWritableDir(f.versioner.Archive, f.fs, state.file.Name); err != nil {
				return err
			}
		}
	}

	// Replace the original content with the new one. If it didn't work,
	// leave the temp file in place for reuse.
	if err := osutil.TryRename(f.fs, state.tempName, state.file.Name); err != nil {
		return err
	}

	// Set the correct timestamp on the new file
	f.fs.Chtimes(state.file.Name, state.file.ModTime(), state.file.ModTime()) // never fails

	// Record the updated file in the index
	dbUpdateChan <- dbUpdateJob{state.file, dbUpdateHandleFile}
	return nil
}

func (f *sendReceiveFolder) finisherRoutine(ignores *ignore.Matcher, in <-chan *sharedPullerState, dbUpdateChan chan<- dbUpdateJob, scanChan chan<- string) {
	for state := range in {
		if closed, err := state.finalClose(); closed {
			l.Debugln(f, "closing", state.file.Name)

			f.queue.Done(state.file.Name)

			if err == nil {
				err = f.performFinish(ignores, state, dbUpdateChan, scanChan)
			}

			if err != nil {
				f.newError("finisher", state.file.Name, err)
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

			events.Default.Log(events.ItemFinished, map[string]interface{}{
				"folder": f.folderID,
				"item":   state.file.Name,
				"error":  events.Error(err),
				"type":   "file",
				"action": "update",
			})

			if f.model.progressEmitter != nil {
				f.model.progressEmitter.Deregister(state)
			}
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

	batch := make([]dbUpdateJob, 0, maxBatchSizeFiles)
	files := make([]protocol.FileInfo, 0, maxBatchSizeFiles)
	tick := time.NewTicker(maxBatchTime)
	defer tick.Stop()

	changedDirs := make(map[string]struct{})

	handleBatch := func() {
		found := false
		var lastFile protocol.FileInfo

		for _, job := range batch {
			files = append(files, job.file)

			switch job.jobType {
			case dbUpdateHandleFile, dbUpdateShortcutFile:
				changedDirs[filepath.Dir(job.file.Name)] = struct{}{}
			case dbUpdateHandleDir:
				changedDirs[job.file.Name] = struct{}{}
			case dbUpdateHandleSymlink, dbUpdateInvalidate:
				// fsyncing symlinks is only supported by MacOS
				// and invalidated files are db only changes -> no sync
			}

			if job.file.IsInvalid() || (job.file.IsDirectory() && !job.file.IsSymlink()) {
				continue
			}

			if job.jobType&(dbUpdateHandleFile|dbUpdateDeleteFile) == 0 {
				continue
			}

			found = true
			lastFile = job.file
		}

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
		f.model.updateLocalsFromPulling(f.folderID, files)

		if found {
			f.model.receivedFile(f.folderID, lastFile)
		}

		batch = batch[:0]
		files = files[:0]
	}

	batchSizeBytes := 0
loop:
	for {
		select {
		case job, ok := <-dbUpdateChan:
			if !ok {
				break loop
			}

			job.file.Sequence = 0
			batch = append(batch, job)

			batchSizeBytes += job.file.ProtoSize()
			if len(batch) == maxBatchSizeFiles || batchSizeBytes > maxBatchSizeBytes {
				handleBatch()
				batchSizeBytes = 0
			}

		case <-tick.C:
			if len(batch) > 0 {
				handleBatch()
				batchSizeBytes = 0
			}
		}
	}

	if len(batch) > 0 {
		handleBatch()
	}
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

func (f *sendReceiveFolder) moveForConflict(name string, lastModBy string) error {
	if strings.Contains(filepath.Base(name), ".sync-conflict-") {
		l.Infoln("Conflict for", name, "which is already a conflict copy; not copying again.")
		if err := f.fs.Remove(name); err != nil && !fs.IsNotExist(err) {
			return err
		}
		return nil
	}

	if f.MaxConflicts == 0 {
		if err := f.fs.Remove(name); err != nil && !fs.IsNotExist(err) {
			return err
		}
		return nil
	}

	ext := filepath.Ext(name)
	withoutExt := name[:len(name)-len(ext)]
	newName := withoutExt + time.Now().Format(".sync-conflict-20060102-150405-") + lastModBy + ext
	err := f.fs.Rename(name, newName)
	if fs.IsNotExist(err) {
		// We were supposed to move a file away but it does not exist. Either
		// the user has already moved it away, or the conflict was between a
		// remote modification and a local delete. In either way it does not
		// matter, go ahead as if the move succeeded.
		err = nil
	}
	if f.MaxConflicts > -1 {
		matches, gerr := f.fs.Glob(withoutExt + ".sync-conflict-????????-??????*" + ext)
		if gerr == nil && len(matches) > f.MaxConflicts {
			sort.Sort(sort.Reverse(sort.StringSlice(matches)))
			for _, match := range matches[f.MaxConflicts:] {
				gerr = f.fs.Remove(match)
				if gerr != nil {
					l.Debugln(f, "removing extra conflict", gerr)
				}
			}
		} else if gerr != nil {
			l.Debugln(f, "globbing for conflicts", gerr)
		}
	}
	return err
}

func (f *sendReceiveFolder) newError(context, path string, err error) {
	f.errorsMut.Lock()
	defer f.errorsMut.Unlock()

	// We might get more than one error report for a file (i.e. error on
	// Write() followed by Close()); we keep the first error as that is
	// probably closer to the root cause.
	if _, ok := f.errors[path]; ok {
		return
	}
	l.Infof("Puller (folder %s, file %q): %s: %v", f.Description(), path, context, err)
	f.errors[path] = fmt.Sprintf("%s: %s", context, err.Error())
}

func (f *sendReceiveFolder) clearErrors() {
	f.errorsMut.Lock()
	f.errors = make(map[string]string)
	f.errorsMut.Unlock()
}

func (f *sendReceiveFolder) PullErrors() []FileError {
	f.errorsMut.Lock()
	errors := make([]FileError, 0, len(f.errors))
	for path, err := range f.errors {
		errors = append(errors, FileError{path, err})
	}
	sort.Sort(fileErrorList(errors))
	f.errorsMut.Unlock()
	return errors
}

// deleteDir attempts to delete a directory. It checks for files/dirs inside
// the directory and removes them if possible or returns an error if it fails
func (f *sendReceiveFolder) deleteDir(dir string, ignores *ignore.Matcher, scanChan chan<- string) error {
	files, _ := f.fs.DirNames(dir)

	toBeDeleted := make([]string, 0, len(files))

	hasIgnored := false
	hasKnown := false
	hasToBeScanned := false

	for _, dirFile := range files {
		fullDirFile := filepath.Join(dir, dirFile)
		if fs.IsTemporary(dirFile) || ignores.Match(fullDirFile).IsDeletable() {
			toBeDeleted = append(toBeDeleted, fullDirFile)
		} else if ignores != nil && ignores.Match(fullDirFile).IsIgnored() {
			hasIgnored = true
		} else if cf, ok := f.model.CurrentFolderFile(f.ID, fullDirFile); !ok || cf.IsDeleted() || cf.IsInvalid() {
			// Something appeared in the dir that we either are not
			// aware of at all, that we think should be deleted or that
			// is invalid, but not currently ignored -> schedule scan
			scanChan <- fullDirFile
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

// byComponentCount sorts by the number of path components in Name, that is
// "x/y" sorts before "foo/bar/baz".
type byComponentCount []protocol.FileInfo

func (l byComponentCount) Len() int {
	return len(l)
}

func (l byComponentCount) Less(a, b int) bool {
	return componentCount(l[a].Name) < componentCount(l[b].Name)
}

func (l byComponentCount) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}

func componentCount(name string) int {
	count := 0
	for _, codepoint := range name {
		if codepoint == fs.PathSeparator {
			count++
		}
	}
	return count
}

type byteSemaphore struct {
	max       int
	available int
	mut       stdsync.Mutex
	cond      *stdsync.Cond
}

func newByteSemaphore(max int) *byteSemaphore {
	s := byteSemaphore{
		max:       max,
		available: max,
	}
	s.cond = stdsync.NewCond(&s.mut)
	return &s
}

func (s *byteSemaphore) take(bytes int) {
	if bytes > s.max {
		panic("bug: more than max bytes will never be available")
	}
	s.mut.Lock()
	for bytes > s.available {
		s.cond.Wait()
	}
	s.available -= bytes
	s.mut.Unlock()
}

func (s *byteSemaphore) give(bytes int) {
	s.mut.Lock()
	if s.available+bytes > s.max {
		panic("bug: can never give more than max")
	}
	s.available += bytes
	s.cond.Broadcast()
	s.mut.Unlock()
}

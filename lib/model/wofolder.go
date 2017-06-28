// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/versioner"
)

func init() {
	folderFactories[config.FolderTypeReceiveOnly] = newReceiveOnlyFolder
}

type receiveOnlyFolder struct {
	sendReceiveFolder
}

func newReceiveOnlyFolder(model *Model, cfg config.FolderConfiguration, ver versioner.Versioner, mtimeFS *fs.MtimeFS) service {

	ctx, cancel := context.WithCancel(context.Background())

	f := &receiveOnlyFolder{
		sendReceiveFolder{
			folder: folder{
				stateTracker:        newStateTracker(cfg.ID),
				scan:                newFolderScanner(cfg),
				ctx:                 ctx,
				cancel:              cancel,
				model:               model,
				initialScanFinished: make(chan struct{}),
			},
			FolderConfiguration: cfg,

			mtimeFS:   mtimeFS,
			dir:       cfg.Path(),
			versioner: ver,

			queue:       newJobQueue(),
			pullTimer:   time.NewTimer(time.Second),
			remoteIndex: make(chan struct{}, 1), // This needs to be 1-buffered so that we queue a notification if we're busy doing a pull when it comes.

			errorsMut: sync.NewMutex(),
		},
	}

	f.configureCopiersAndPullers()
	return f
}

func (f *receiveOnlyFolder) PullFile(files []protocol.FileInfo) {
	pullChan := make(chan pullBlockState)
	copyChan := make(chan copyBlocksState)
	finisherChan := make(chan *sharedPullerState)

	updateWg := sync.NewWaitGroup()
	copyWg := sync.NewWaitGroup()
	pullWg := sync.NewWaitGroup()
	doneWg := sync.NewWaitGroup()

	l.Debugln("In PullFile:::", files)

	l.Debugln(f, "c", f.Copiers, "p", f.Pullers)

	f.dbUpdates = make(chan dbUpdateJob)
	updateWg.Add(1)
	go func() {
		// dbUpdaterRoutine finishes when f.dbUpdates is closed
		f.dbUpdaterRoutine()
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

	for i := 0; i < f.Pullers; i++ {
		pullWg.Add(1)
		go func() {
			// pullerRoutine finishes when pullChan is closed
			f.pullerRoutine(pullChan, finisherChan)
			pullWg.Done()
		}()
	}

	doneWg.Add(1)
	// finisherRoutine finishes when finisherChan is closed
	go func() {
		f.finisherRoutine(finisherChan)
		doneWg.Done()
	}()
	for _, fi := range files {
		switch {
		case fi.IsDirectory() && !fi.IsSymlink():
			l.Debugln("It is a directory local change")
			f.handleDirReceiveOnly(fi)
		case fi.IsSymlink():
			l.Debugln("It is a sym link local change")
			f.handleSymlinkReceiveOnly(fi)
		default:
			l.Debugln("It is a file local change")
			f.handleFileReceiveOnly(fi, copyChan, finisherChan)
		}
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

	// Wait for db updates to complete
	close(f.dbUpdates)
	updateWg.Wait()

	return
}

func (f *receiveOnlyFolder) handleFileReceiveOnly(curFile protocol.FileInfo, copyChan chan<- copyBlocksState, finisherChan chan<- *sharedPullerState) {
	file, hasCurFile := f.model.CurrentFolderFile(f.folderID, curFile.Name)

	if !hasCurFile {
		l.Debugln(file, " has been created, has to be deleted")
		return
	}
	have, need := scanner.BlockDiff(curFile.Blocks, file.Blocks)

	l.Debugln("let's see what's happening:1::", curFile, hasCurFile, have, need, file)

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
			l.Infoln("Puller: shortcut:", err)
			f.newError(file.Name, err)
		} else {
			f.dbUpdates <- dbUpdateJob{file, dbUpdateShortcutFile}
		}

		return
	}

	l.Debugln("let's see what's happening:2::", curFile, hasCurFile, have, need, file)
	// Figure out the absolute filenames we need once and for all
	tempName, err := rootedJoinedPath(f.dir, ignore.TempName(file.Name))
	if err != nil {
		f.newError(file.Name, err)
		return
	}
	realName, err := rootedJoinedPath(f.dir, file.Name)
	if err != nil {
		f.newError(file.Name, err)
		return
	}
	l.Debugln("let's see what's happening:1::", realName)

	if hasCurFile && !curFile.IsDirectory() && !curFile.IsSymlink() {
		// Check that the file on disk is what we expect it to be according to
		// the database. If there's a mismatch here, there might be local
		// changes that we don't know about yet and we should scan before
		// touching the file. If we can't stat the file we'll just pull it.
		l.Debugln("let's see what's happening:2::")
		if info, err := f.mtimeFS.Lstat(realName); err == nil {
			if !info.ModTime().Equal(curFile.ModTime()) || info.Size() != curFile.Size {
				l.Debugln("file modified but not rescanned; not pulling:", realName)
				// Scan() is synchronous (i.e. blocks until the scan is
				// completed and returns an error), but a scan can't happen
				// while we're in the puller routine. Request the scan in the
				// background and it'll be handled when the current pulling
				// sweep is complete. As we do retries, we'll queue the scan
				// for this file up to ten times, but the last nine of those
				// scans will be cheap...
				go f.Scan([]string{file.Name})
				return
			}
		}
	}

	scanner.PopulateOffsets(file.Blocks)

	var blocks []protocol.BlockInfo
	var blocksSize int64
	var reused []int32

	// Check for an old temporary file which might have some blocks we could
	// reuse.
	tempBlocks, err := scanner.HashFile(f.ctx, fs.DefaultFilesystem, tempName, protocol.BlockSize, nil, false)
	l.Debugln("let's see what's happening:2::", tempBlocks)
	if err == nil {
		// Check for any reusable blocks in the temp file
		tempCopyBlocks, _ := scanner.BlockDiff(tempBlocks, file.Blocks)

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
			osutil.InWritableDir(os.Remove, tempName)
		}
	} else {
		// Copy the blocks, as we don't want to shuffle them on the FileInfo
		blocks = append(blocks, file.Blocks...)
		blocksSize = file.Size
	}

	if f.MinDiskFree.BaseValue() > 0 {
		if free, err := osutil.DiskFreeBytes(f.dir); err == nil && free < blocksSize {
			l.Warnf(`Folder "%s": insufficient disk space in %s for %s: have %.2f MiB, need %.2f MiB`, f.folderID, f.dir, file.Name, float64(free)/1024/1024, float64(blocksSize)/1024/1024)
			f.newError(file.Name, errors.New("insufficient space"))
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
		folder:           f.folderID,
		tempName:         tempName,
		realName:         realName,
		copyTotal:        len(blocks),
		copyNeeded:       len(blocks),
		reused:           len(reused),
		updated:          time.Now(),
		available:        reused,
		availableUpdated: time.Now(),
		ignorePerms:      f.ignorePermissions(file),
		version:          curFile.Version,
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

	l.Debugln("let's see what's happening:1::", cs)
}

func (f *receiveOnlyFolder) handleDirReceiveOnly(file protocol.FileInfo) {
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

	realName, err := rootedJoinedPath(f.dir, file.Name)
	if err != nil {
		f.newError(file.Name, err)
		return
	}
	mode := os.FileMode(file.Permissions & 0777)
	if f.ignorePermissions(file) {
		mode = 0777
	}

	file, _ = f.model.CurrentFolderFile(f.folderID, file.Name)

	if shouldDebug() {
		curFile, _ := f.model.CurrentFolderFile(f.folderID, file.Name)
		l.Debugf("need dir\n\t%v\n\t%v", file, curFile)
	}

	info, err := f.mtimeFS.Lstat(realName)
	switch {
	// There is already something under that name, but it's a file/link.
	// Most likely a file/link is getting replaced with a directory.
	// Remove the file/link and fall through to directory creation.
	case err == nil && (!info.IsDir() || info.IsSymlink()):
		err = osutil.InWritableDir(os.Remove, realName)
		if err != nil {
			l.Infof("Puller (folder %q, dir %q): %v", f.folderID, file.Name, err)
			f.newError(file.Name, err)
			return
		}
		fallthrough
	// The directory doesn't exist, so we create it with the right
	// mode bits from the start.
	case err != nil && os.IsNotExist(err):
		// We declare a function that acts on only the path name, so
		// we can pass it to InWritableDir. We use a regular Mkdir and
		// not MkdirAll because the parent should already exist.
		mkdir := func(path string) error {
			err = os.Mkdir(path, mode)
			if err != nil || f.ignorePermissions(file) {
				return err
			}

			// Stat the directory so we can check its permissions.
			info, err := f.mtimeFS.Lstat(path)
			if err != nil {
				return err
			}

			// Mask for the bits we want to preserve and add them in to the
			// directories permissions.
			return os.Chmod(path, mode|(os.FileMode(info.Mode())&retainBits))
		}

		if err = osutil.InWritableDir(mkdir, realName); err == nil {
			f.dbUpdates <- dbUpdateJob{file, dbUpdateHandleDir}
		} else {
			l.Infof("Puller (folder %q, dir %q): %v", f.folderID, file.Name, err)
			f.newError(file.Name, err)
		}
		return
	// Weird error when stat()'ing the dir. Probably won't work to do
	// anything else with it if we can't even stat() it.
	case err != nil:
		l.Infof("Puller (folder %q, dir %q): %v", f.folderID, file.Name, err)
		f.newError(file.Name, err)
		return
	}

	// The directory already exists, so we just correct the mode bits. (We
	// don't handle modification times on directories, because that sucks...)
	// It's OK to change mode bits on stuff within non-writable directories.
	if f.ignorePermissions(file) {
		f.dbUpdates <- dbUpdateJob{file, dbUpdateHandleDir}
	} else if err := os.Chmod(realName, mode|(os.FileMode(info.Mode())&retainBits)); err == nil {
		f.dbUpdates <- dbUpdateJob{file, dbUpdateHandleDir}
	} else {
		l.Infof("Puller (folder %q, dir %q): %v", f.folderID, file.Name, err)
		f.newError(file.Name, err)
	}
}

func (f *receiveOnlyFolder) handleSymlinkReceiveOnly(file protocol.FileInfo) {
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

	realName, err := rootedJoinedPath(f.dir, file.Name)
	if err != nil {
		f.newError(file.Name, err)
		return
	}

	file, _ = f.model.CurrentFolderFile(f.folderID, file.Name)

	if shouldDebug() {
		curFile, _ := f.model.CurrentFolderFile(f.folderID, file.Name)
		l.Debugf("need symlink\n\t%v\n\t%v", file, curFile)
	}

	if len(file.SymlinkTarget) == 0 {
		// Index entry from a Syncthing predating the support for including
		// the link target in the index entry. We log this as an error.
		err = errors.New("incompatible symlink entry; rescan with newer Syncthing on source")
		l.Infof("Puller (folder %q, dir %q): %v", f.folderID, file.Name, err)
		f.newError(file.Name, err)
		return
	}

	if _, err = f.mtimeFS.Lstat(realName); err == nil {
		// There is already something under that name. Remove it to replace
		// with the symlink. This also handles the "change symlink type"
		// path.
		err = osutil.InWritableDir(os.Remove, realName)
		if err != nil {
			l.Infof("Puller (folder %q, dir %q): %v", f.folderID, file.Name, err)
			f.newError(file.Name, err)
			return
		}
	}

	// We declare a function that acts on only the path name, so
	// we can pass it to InWritableDir.
	createLink := func(path string) error {
		return os.Symlink(file.SymlinkTarget, path)
	}

	if err = osutil.InWritableDir(createLink, realName); err == nil {
		f.dbUpdates <- dbUpdateJob{file, dbUpdateHandleSymlink}
	} else {
		l.Infof("Puller (folder %q, dir %q): %v", f.folderID, file.Name, err)
		f.newError(file.Name, err)
	}
}

func (f *receiveOnlyFolder) deleteFileReceiveOnly(file protocol.FileInfo) {
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

	realName, err := rootedJoinedPath(f.dir, file.Name)
	if err != nil {
		f.newError(file.Name, err)
		return
	}

	err = osutil.InWritableDir(os.Remove, realName)
	if err != nil {
		l.Debugln("Something Went Wrong In Deleting the file: ", file)
	}
}

// deleteDir attempts to delete the given directory
func (f *receiveOnlyFolder) deleteDirReceiveOnly(file protocol.FileInfo, matcher *ignore.Matcher) {
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

	realName, err := rootedJoinedPath(f.dir, file.Name)
	if err != nil {
		f.newError(file.Name, err)
		return
	}

	// Delete any temporary files lying around in the directory
	dir, _ := os.Open(realName)
	if dir != nil {
		files, _ := dir.Readdirnames(-1)
		for _, dirFile := range files {
			fullDirFile := filepath.Join(file.Name, dirFile)
			if ignore.IsTemporary(dirFile) || (matcher != nil &&
				matcher.Match(fullDirFile).IsDeletable()) {
				os.RemoveAll(filepath.Join(f.dir, fullDirFile))
			}
		}
		dir.Close()
	}

	err = osutil.InWritableDir(os.Remove, realName)
}

func (f *receiveOnlyFolder) deletions(files []protocol.FileInfo) {

	updateWg := sync.NewWaitGroup()

	f.dbUpdates = make(chan dbUpdateJob)
	updateWg.Add(1)
	go func() {
		// dbUpdaterRoutine finishes when f.dbUpdates is closed
		f.dbUpdaterRoutine()
		updateWg.Done()
	}()

	for _, fi := range files {
		fi.Deleted = true
		switch {
		case fi.IsDirectory() && !fi.IsSymlink():
			f.deleteDirReceiveOnly(fi, nil)
		default:
			f.deleteFileReceiveOnly(fi)
		}
	}

	// Wait for db updates to complete
	close(f.dbUpdates)
	updateWg.Wait()
}

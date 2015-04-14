// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/internal/config"
	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/events"
	"github.com/syncthing/syncthing/internal/ignore"
	"github.com/syncthing/syncthing/internal/osutil"
	"github.com/syncthing/syncthing/internal/scanner"
	"github.com/syncthing/syncthing/internal/symlinks"
	"github.com/syncthing/syncthing/internal/versioner"
)

// TODO: Stop on errors

const (
	pauseIntv     = 60 * time.Second
	nextPullIntv  = 10 * time.Second
	checkPullIntv = 1 * time.Second
)

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
}

var (
	activity    = newDeviceActivity()
	errNoDevice = errors.New("no available source device")
)

type rwFolder struct {
	stateTracker

	model           *Model
	progressEmitter *ProgressEmitter

	folder        string
	dir           string
	scanIntv      time.Duration
	versioner     versioner.Versioner
	ignorePerms   bool
	lenientMtimes bool
	copiers       int
	pullers       int
	shortID       uint64

	stop      chan struct{}
	queue     *jobQueue
	dbUpdates chan protocol.FileInfo
}

func newRWFolder(m *Model, shortID uint64, cfg config.FolderConfiguration) *rwFolder {
	return &rwFolder{
		stateTracker: stateTracker{folder: cfg.ID},

		model:           m,
		progressEmitter: m.progressEmitter,

		folder:        cfg.ID,
		dir:           cfg.Path(),
		scanIntv:      time.Duration(cfg.RescanIntervalS) * time.Second,
		ignorePerms:   cfg.IgnorePerms,
		lenientMtimes: cfg.LenientMtimes,
		copiers:       cfg.Copiers,
		pullers:       cfg.Pullers,
		shortID:       shortID,

		stop:  make(chan struct{}),
		queue: newJobQueue(),
	}
}

// Serve will run scans and pulls. It will return when Stop()ed or on a
// critical error.
func (p *rwFolder) Serve() {
	if debug {
		l.Debugln(p, "starting")
		defer l.Debugln(p, "exiting")
	}

	pullTimer := time.NewTimer(checkPullIntv)
	scanTimer := time.NewTimer(time.Millisecond) // The first scan should be done immediately.

	defer func() {
		pullTimer.Stop()
		scanTimer.Stop()
		// TODO: Should there be an actual FolderStopped state?
		p.setState(FolderIdle)
	}()

	var prevVer int64
	var prevIgnoreHash string

	rescheduleScan := func() {
		// Sleep a random time between 3/4 and 5/4 of the configured interval.
		sleepNanos := (p.scanIntv.Nanoseconds()*3 + rand.Int63n(2*p.scanIntv.Nanoseconds())) / 4
		intv := time.Duration(sleepNanos) * time.Nanosecond

		if debug {
			l.Debugln(p, "next rescan in", intv)
		}
		scanTimer.Reset(intv)
	}

	// We don't start pulling files until a scan has been completed.
	initialScanCompleted := false

	for {
		select {
		case <-p.stop:
			return

		// TODO: We could easily add a channel here for notifications from
		// Index(), so that we immediately start a pull when new index
		// information is available. Before that though, I'd like to build a
		// repeatable benchmark of how long it takes to sync a change from
		// device A to device B, so we have something to work against.
		case <-pullTimer.C:
			if !initialScanCompleted {
				if debug {
					l.Debugln(p, "skip (initial)")
				}
				pullTimer.Reset(nextPullIntv)
				continue
			}

			p.model.fmut.RLock()
			curIgnores := p.model.folderIgnores[p.folder]
			p.model.fmut.RUnlock()

			if newHash := curIgnores.Hash(); newHash != prevIgnoreHash {
				// The ignore patterns have changed. We need to re-evaluate if
				// there are files we need now that were ignored before.
				if debug {
					l.Debugln(p, "ignore patterns have changed, resetting prevVer")
				}
				prevVer = 0
				prevIgnoreHash = newHash
			}

			// RemoteLocalVersion() is a fast call, doesn't touch the database.
			curVer := p.model.RemoteLocalVersion(p.folder)
			if curVer == prevVer {
				if debug {
					l.Debugln(p, "skip (curVer == prevVer)", prevVer)
				}
				pullTimer.Reset(checkPullIntv)
				continue
			}

			if debug {
				l.Debugln(p, "pulling", prevVer, curVer)
			}
			p.setState(FolderSyncing)
			tries := 0
			for {
				tries++

				changed := p.pullerIteration(curIgnores)
				if debug {
					l.Debugln(p, "changed", changed)
				}

				if changed == 0 {
					// No files were changed by the puller, so we are in
					// sync. Remember the local version number and
					// schedule a resync a little bit into the future.

					if lv := p.model.RemoteLocalVersion(p.folder); lv < curVer {
						// There's a corner case where the device we needed
						// files from disconnected during the puller
						// iteration. The files will have been removed from
						// the index, so we've concluded that we don't need
						// them, but at the same time we have the local
						// version that includes those files in curVer. So we
						// catch the case that localVersion might have
						// decreased here.
						l.Debugln(p, "adjusting curVer", lv)
						curVer = lv
					}
					prevVer = curVer
					if debug {
						l.Debugln(p, "next pull in", nextPullIntv)
					}
					pullTimer.Reset(nextPullIntv)
					break
				}

				if tries > 10 {
					// We've tried a bunch of times to get in sync, but
					// we're not making it. Probably there are write
					// errors preventing us. Flag this with a warning and
					// wait a bit longer before retrying.
					l.Warnf("Folder %q isn't making progress - check logs for possible root cause. Pausing puller for %v.", p.folder, pauseIntv)
					if debug {
						l.Debugln(p, "next pull in", pauseIntv)
					}
					pullTimer.Reset(pauseIntv)
					break
				}
			}
			p.setState(FolderIdle)

		// The reason for running the scanner from within the puller is that
		// this is the easiest way to make sure we are not doing both at the
		// same time.
		case <-scanTimer.C:
			if err := p.model.CheckFolderHealth(p.folder); err != nil {
				l.Infoln("Skipping folder", p.folder, "scan due to folder error:", err)
				rescheduleScan()
				continue
			}

			if debug {
				l.Debugln(p, "rescan")
			}

			if err := p.model.ScanFolder(p.folder); err != nil {
				// Potentially sets the error twice, once in the scanner just
				// by doing a check, and once here, if the error returned is
				// the same one as returned by CheckFolderHealth, though
				// duplicate set is handled by setError.
				p.setError(err)
				rescheduleScan()
				continue
			}

			if p.scanIntv > 0 {
				rescheduleScan()
			}
			if !initialScanCompleted {
				l.Infoln("Completed initial scan (rw) of folder", p.folder)
				initialScanCompleted = true
			}
		}
	}
}

func (p *rwFolder) Stop() {
	close(p.stop)
}

func (p *rwFolder) String() string {
	return fmt.Sprintf("rwFolder/%s@%p", p.folder, p)
}

// pullerIteration runs a single puller iteration for the given folder and
// returns the number items that should have been synced (even those that
// might have failed). One puller iteration handles all files currently
// flagged as needed in the folder.
func (p *rwFolder) pullerIteration(ignores *ignore.Matcher) int {
	pullChan := make(chan pullBlockState)
	copyChan := make(chan copyBlocksState)
	finisherChan := make(chan *sharedPullerState)

	var updateWg sync.WaitGroup
	var copyWg sync.WaitGroup
	var pullWg sync.WaitGroup
	var doneWg sync.WaitGroup

	if debug {
		l.Debugln(p, "c", p.copiers, "p", p.pullers)
	}

	p.dbUpdates = make(chan protocol.FileInfo)
	updateWg.Add(1)
	go func() {
		// dbUpdaterRoutine finishes when p.dbUpdates is closed
		p.dbUpdaterRoutine()
		updateWg.Done()
	}()

	for i := 0; i < p.copiers; i++ {
		copyWg.Add(1)
		go func() {
			// copierRoutine finishes when copyChan is closed
			p.copierRoutine(copyChan, pullChan, finisherChan)
			copyWg.Done()
		}()
	}

	for i := 0; i < p.pullers; i++ {
		pullWg.Add(1)
		go func() {
			// pullerRoutine finishes when pullChan is closed
			p.pullerRoutine(pullChan, finisherChan)
			pullWg.Done()
		}()
	}

	doneWg.Add(1)
	// finisherRoutine finishes when finisherChan is closed
	go func() {
		p.finisherRoutine(finisherChan)
		doneWg.Done()
	}()

	p.model.fmut.RLock()
	folderFiles := p.model.folderFiles[p.folder]
	p.model.fmut.RUnlock()

	// !!!
	// WithNeed takes a database snapshot (by necessity). By the time we've
	// handled a bunch of files it might have become out of date and we might
	// be attempting to sync with an old version of a file...
	// !!!

	changed := 0

	fileDeletions := map[string]protocol.FileInfo{}
	dirDeletions := []protocol.FileInfo{}
	buckets := map[string][]protocol.FileInfo{}

	folderFiles.WithNeed(protocol.LocalDeviceID, func(intf db.FileIntf) bool {

		// Needed items are delivered sorted lexicographically. This isn't
		// really optimal from a performance point of view - it would be
		// better if files were handled in random order, to spread the load
		// over the cluster. But it means that we can be sure that we fully
		// handle directories before the files that go inside them, which is
		// nice.

		file := intf.(protocol.FileInfo)

		if ignores.Match(file.Name) {
			// This is an ignored file. Skip it, continue iteration.
			return true
		}

		if debug {
			l.Debugln(p, "handling", file.Name)
		}

		switch {
		case file.IsDeleted():
			// A deleted file, directory or symlink
			if file.IsDirectory() {
				dirDeletions = append(dirDeletions, file)
			} else {
				fileDeletions[file.Name] = file
				df, ok := p.model.CurrentFolderFile(p.folder, file.Name)
				// Local file can be already deleted, but with a lower version
				// number, hence the deletion coming in again as part of
				// WithNeed, furthermore, the file can simply be of the wrong
				// type if we haven't yet managed to pull it.
				if ok && !df.IsDeleted() && !df.IsSymlink() && !df.IsDirectory() {
					// Put files into buckets per first hash
					key := string(df.Blocks[0].Hash)
					buckets[key] = append(buckets[key], df)
				}
			}
		case file.IsDirectory() && !file.IsSymlink():
			// A new or changed directory
			if debug {
				l.Debugln("Creating directory", file.Name)
			}
			p.handleDir(file)
		default:
			// A new or changed file or symlink. This is the only case where we
			// do stuff concurrently in the background
			p.queue.Push(file.Name)
		}

		changed++
		return true
	})

nextFile:
	for {
		fileName, ok := p.queue.Pop()
		if !ok {
			break
		}

		f, ok := p.model.CurrentGlobalFile(p.folder, fileName)
		if !ok {
			// File is no longer in the index. Mark it as done and drop it.
			p.queue.Done(fileName)
			continue
		}

		// Local file can be already deleted, but with a lower version
		// number, hence the deletion coming in again as part of
		// WithNeed, furthermore, the file can simply be of the wrong type if
		// the global index changed while we were processing this iteration.
		if !f.IsDeleted() && !f.IsSymlink() && !f.IsDirectory() {
			key := string(f.Blocks[0].Hash)
			for i, candidate := range buckets[key] {
				if scanner.BlocksEqual(candidate.Blocks, f.Blocks) {
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

					p.renameFile(desired, f)

					p.queue.Done(fileName)
					continue nextFile
				}
			}
		}

		// Not a rename or a symlink, deal with it.
		p.handleFile(f, copyChan, finisherChan)
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
		if debug {
			l.Debugln("Deleting file", file.Name)
		}
		p.deleteFile(file)
	}

	for i := range dirDeletions {
		dir := dirDeletions[len(dirDeletions)-i-1]
		if debug {
			l.Debugln("Deleting dir", dir.Name)
		}
		p.deleteDir(dir)
	}

	// Wait for db updates to complete
	close(p.dbUpdates)
	updateWg.Wait()

	return changed
}

// handleDir creates or updates the given directory
func (p *rwFolder) handleDir(file protocol.FileInfo) {
	var err error
	events.Default.Log(events.ItemStarted, map[string]interface{}{
		"folder":  p.folder,
		"item":    file.Name,
		"details": db.ToTruncated(file),
	})
	defer func() {
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"folder": p.folder,
			"item":   file.Name,
			"error":  err,
		})
	}()

	realName := filepath.Join(p.dir, file.Name)
	mode := os.FileMode(file.Flags & 0777)
	if p.ignorePerms {
		mode = 0755
	}

	if debug {
		curFile, _ := p.model.CurrentFolderFile(p.folder, file.Name)
		l.Debugf("need dir\n\t%v\n\t%v", file, curFile)
	}

	info, err := osutil.Lstat(realName)
	switch {
	// There is already something under that name, but it's a file/link.
	// Most likely a file/link is getting replaced with a directory.
	// Remove the file/link and fall through to directory creation.
	case err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0):
		err = osutil.InWritableDir(os.Remove, realName)
		if err != nil {
			l.Infof("Puller (folder %q, dir %q): %v", p.folder, file.Name, err)
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
			if err != nil || p.ignorePerms {
				return err
			}
			return os.Chmod(path, mode)
		}

		if err = osutil.InWritableDir(mkdir, realName); err == nil {
			p.dbUpdates <- file
		} else {
			l.Infof("Puller (folder %q, dir %q): %v", p.folder, file.Name, err)
		}
		return
	// Weird error when stat()'ing the dir. Probably won't work to do
	// anything else with it if we can't even stat() it.
	case err != nil:
		l.Infof("Puller (folder %q, dir %q): %v", p.folder, file.Name, err)
		return
	}

	// The directory already exists, so we just correct the mode bits. (We
	// don't handle modification times on directories, because that sucks...)
	// It's OK to change mode bits on stuff within non-writable directories.

	if p.ignorePerms {
		p.dbUpdates <- file
	} else if err := os.Chmod(realName, mode); err == nil {
		p.dbUpdates <- file
	} else {
		l.Infof("Puller (folder %q, dir %q): %v", p.folder, file.Name, err)
	}
}

// deleteDir attempts to delete the given directory
func (p *rwFolder) deleteDir(file protocol.FileInfo) {
	var err error
	events.Default.Log(events.ItemStarted, map[string]interface{}{
		"folder":  p.folder,
		"item":    file.Name,
		"details": db.ToTruncated(file),
	})
	defer func() {
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"folder": p.folder,
			"item":   file.Name,
			"error":  err,
		})
	}()

	realName := filepath.Join(p.dir, file.Name)
	// Delete any temporary files lying around in the directory
	dir, _ := os.Open(realName)
	if dir != nil {
		files, _ := dir.Readdirnames(-1)
		for _, file := range files {
			if defTempNamer.IsTemporary(file) {
				osutil.InWritableDir(os.Remove, filepath.Join(realName, file))
			}
		}
	}
	err = osutil.InWritableDir(os.Remove, realName)
	if err == nil || os.IsNotExist(err) {
		p.dbUpdates <- file
	} else {
		l.Infof("Puller (folder %q, dir %q): delete: %v", p.folder, file.Name, err)
	}
}

// deleteFile attempts to delete the given file
func (p *rwFolder) deleteFile(file protocol.FileInfo) {
	var err error
	events.Default.Log(events.ItemStarted, map[string]interface{}{
		"folder":  p.folder,
		"item":    file.Name,
		"details": db.ToTruncated(file),
	})
	defer func() {
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"folder": p.folder,
			"item":   file.Name,
			"error":  err,
		})
	}()

	realName := filepath.Join(p.dir, file.Name)

	cur, ok := p.model.CurrentFolderFile(p.folder, file.Name)
	if ok && p.inConflict(cur.Version, file.Version) {
		// There is a conflict here. Move the file to a conflict copy instead
		// of deleting. Also merge with the version vector we had, to indicate
		// we have resolved the conflict.
		file.Version = file.Version.Merge(cur.Version)
		err = osutil.InWritableDir(moveForConflict, realName)
	} else if p.versioner != nil {
		err = osutil.InWritableDir(p.versioner.Archive, realName)
	} else {
		err = osutil.InWritableDir(os.Remove, realName)
	}

	if err != nil && !os.IsNotExist(err) {
		l.Infof("Puller (folder %q, file %q): delete: %v", p.folder, file.Name, err)
	} else {
		p.dbUpdates <- file
	}
}

// renameFile attempts to rename an existing file to a destination
// and set the right attributes on it.
func (p *rwFolder) renameFile(source, target protocol.FileInfo) {
	var err error
	events.Default.Log(events.ItemStarted, map[string]interface{}{
		"folder":  p.folder,
		"item":    source.Name,
		"details": db.ToTruncated(source),
	})
	events.Default.Log(events.ItemStarted, map[string]interface{}{
		"folder":  p.folder,
		"item":    target.Name,
		"details": db.ToTruncated(source),
	})
	defer func() {
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"folder": p.folder,
			"item":   source.Name,
			"error":  err,
		})
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"folder": p.folder,
			"item":   target.Name,
			"error":  err,
		})
	}()

	if debug {
		l.Debugln(p, "taking rename shortcut", source.Name, "->", target.Name)
	}

	from := filepath.Join(p.dir, source.Name)
	to := filepath.Join(p.dir, target.Name)

	if p.versioner != nil {
		err = osutil.Copy(from, to)
		if err == nil {
			err = osutil.InWritableDir(p.versioner.Archive, from)
		}
	} else {
		err = osutil.TryRename(from, to)
	}

	if err == nil {
		// The file was renamed, so we have handled both the necessary delete
		// of the source and the creation of the target. Fix-up the metadata,
		// and update the local index of the target file.

		p.dbUpdates <- source

		err = p.shortcutFile(target)
		if err != nil {
			l.Infof("Puller (folder %q, file %q): rename from %q metadata: %v", p.folder, target.Name, source.Name, err)
			return
		}
	} else {
		// We failed the rename so we have a source file that we still need to
		// get rid of. Attempt to delete it instead so that we make *some*
		// progress. The target is unhandled.

		err = osutil.InWritableDir(os.Remove, from)
		if err != nil {
			l.Infof("Puller (folder %q, file %q): delete %q after failed rename: %v", p.folder, target.Name, source.Name, err)
			return
		}

		p.dbUpdates <- source
	}
}

// handleFile queues the copies and pulls as necessary for a single new or
// changed file.
func (p *rwFolder) handleFile(file protocol.FileInfo, copyChan chan<- copyBlocksState, finisherChan chan<- *sharedPullerState) {
	events.Default.Log(events.ItemStarted, map[string]interface{}{
		"folder":  p.folder,
		"item":    file.Name,
		"details": db.ToTruncated(file),
	})

	curFile, ok := p.model.CurrentFolderFile(p.folder, file.Name)

	if ok && len(curFile.Blocks) == len(file.Blocks) && scanner.BlocksEqual(curFile.Blocks, file.Blocks) {
		// We are supposed to copy the entire file, and then fetch nothing. We
		// are only updating metadata, so we don't actually *need* to make the
		// copy.
		if debug {
			l.Debugln(p, "taking shortcut on", file.Name)
		}
		p.queue.Done(file.Name)
		var err error
		if file.IsSymlink() {
			err = p.shortcutSymlink(file)
		} else {
			err = p.shortcutFile(file)
		}
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"folder": p.folder,
			"item":   file.Name,
			"error":  err,
		})
		return
	}

	scanner.PopulateOffsets(file.Blocks)

	// Figure out the absolute filenames we need once and for all
	tempName := filepath.Join(p.dir, defTempNamer.TempName(file.Name))
	realName := filepath.Join(p.dir, file.Name)

	reused := 0
	var blocks []protocol.BlockInfo

	// Check for an old temporary file which might have some blocks we could
	// reuse.
	tempBlocks, err := scanner.HashFile(tempName, protocol.BlockSize)
	if err == nil {
		// Check for any reusable blocks in the temp file
		tempCopyBlocks, _ := scanner.BlockDiff(tempBlocks, file.Blocks)

		// block.String() returns a string unique to the block
		existingBlocks := make(map[string]struct{}, len(tempCopyBlocks))
		for _, block := range tempCopyBlocks {
			existingBlocks[block.String()] = struct{}{}
		}

		// Since the blocks are already there, we don't need to get them.
		for _, block := range file.Blocks {
			_, ok := existingBlocks[block.String()]
			if !ok {
				blocks = append(blocks, block)
			}
		}

		// The sharedpullerstate will know which flags to use when opening the
		// temp file depending if we are reusing any blocks or not.
		reused = len(file.Blocks) - len(blocks)
		if reused == 0 {
			// Otherwise, discard the file ourselves in order for the
			// sharedpuller not to panic when it fails to exlusively create a
			// file which already exists
			os.Remove(tempName)
		}
	} else {
		blocks = file.Blocks
	}

	s := sharedPullerState{
		file:        file,
		folder:      p.folder,
		tempName:    tempName,
		realName:    realName,
		copyTotal:   len(blocks),
		copyNeeded:  len(blocks),
		reused:      reused,
		ignorePerms: p.ignorePerms,
		version:     curFile.Version,
	}

	if debug {
		l.Debugf("%v need file %s; copy %d, reused %v", p, file.Name, len(blocks), reused)
	}

	cs := copyBlocksState{
		sharedPullerState: &s,
		blocks:            blocks,
	}
	copyChan <- cs
}

// shortcutFile sets file mode and modification time, when that's the only
// thing that has changed.
func (p *rwFolder) shortcutFile(file protocol.FileInfo) (err error) {
	realName := filepath.Join(p.dir, file.Name)
	if !p.ignorePerms {
		err = os.Chmod(realName, os.FileMode(file.Flags&0777))
		if err != nil {
			l.Infof("Puller (folder %q, file %q): shortcut: %v", p.folder, file.Name, err)
			return
		}
	}

	t := time.Unix(file.Modified, 0)
	err = os.Chtimes(realName, t, t)
	if err != nil {
		if p.lenientMtimes {
			err = nil
			// We accept the failure with a warning here and allow the sync to
			// continue. We'll sync the new mtime back to the other devices later.
			// If they have the same problem & setting, we might never get in
			// sync.
			l.Infof("Puller (folder %q, file %q): shortcut: %v (continuing anyway as requested)", p.folder, file.Name, err)
		} else {
			l.Infof("Puller (folder %q, file %q): shortcut: %v", p.folder, file.Name, err)
			return
		}
	}

	// This may have been a conflict. We should merge the version vectors so
	// that our clock doesn't move backwards.
	if cur, ok := p.model.CurrentFolderFile(p.folder, file.Name); ok {
		file.Version = file.Version.Merge(cur.Version)
	}

	p.dbUpdates <- file
	return
}

// shortcutSymlink changes the symlinks type if necessery.
func (p *rwFolder) shortcutSymlink(file protocol.FileInfo) (err error) {
	err = symlinks.ChangeType(filepath.Join(p.dir, file.Name), file.Flags)
	if err == nil {
		p.dbUpdates <- file
	} else {
		l.Infof("Puller (folder %q, file %q): symlink shortcut: %v", p.folder, file.Name, err)
	}
	return
}

// copierRoutine reads copierStates until the in channel closes and performs
// the relevant copies when possible, or passes it to the puller routine.
func (p *rwFolder) copierRoutine(in <-chan copyBlocksState, pullChan chan<- pullBlockState, out chan<- *sharedPullerState) {
	buf := make([]byte, protocol.BlockSize)

	for state := range in {
		if p.progressEmitter != nil {
			p.progressEmitter.Register(state.sharedPullerState)
		}

		dstFd, err := state.tempFile()
		if err != nil {
			// Nothing more to do for this failed file (the error was logged
			// when it happened)
			out <- state.sharedPullerState
			continue
		}

		folderRoots := make(map[string]string)
		p.model.fmut.RLock()
		for folder, cfg := range p.model.folderCfgs {
			folderRoots[folder] = cfg.Path()
		}
		p.model.fmut.RUnlock()

		for _, block := range state.blocks {
			buf = buf[:int(block.Size)]
			found := p.model.finder.Iterate(block.Hash, func(folder, file string, index int32) bool {
				fd, err := os.Open(filepath.Join(folderRoots[folder], file))
				if err != nil {
					return false
				}

				_, err = fd.ReadAt(buf, protocol.BlockSize*int64(index))
				fd.Close()
				if err != nil {
					return false
				}

				hash, err := scanner.VerifyBuffer(buf, block)
				if err != nil {
					if hash != nil {
						if debug {
							l.Debugf("Finder block mismatch in %s:%s:%d expected %q got %q", folder, file, index, block.Hash, hash)
						}
						err = p.model.finder.Fix(folder, file, index, block.Hash, hash)
						if err != nil {
							l.Warnln("finder fix:", err)
						}
					} else if debug {
						l.Debugln("Finder failed to verify buffer", err)
					}
					return false
				}

				_, err = dstFd.WriteAt(buf, block.Offset)
				if err != nil {
					state.fail("dst write", err)
				}
				if file == state.file.Name {
					state.copiedFromOrigin()
				}
				return true
			})

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
				state.copyDone()
			}
		}
		out <- state.sharedPullerState
	}
}

func (p *rwFolder) pullerRoutine(in <-chan pullBlockState, out chan<- *sharedPullerState) {
	for state := range in {
		if state.failed() != nil {
			continue
		}

		// Get an fd to the temporary file. Tehcnically we don't need it until
		// after fetching the block, but if we run into an error here there is
		// no point in issuing the request to the network.
		fd, err := state.tempFile()
		if err != nil {
			continue
		}

		var lastError error
		potentialDevices := p.model.Availability(p.folder, state.file.Name)
		for {
			// Select the least busy device to pull the block from. If we found no
			// feasible device at all, fail the block (and in the long run, the
			// file).
			selected := activity.leastBusy(potentialDevices)
			if selected == (protocol.DeviceID{}) {
				if lastError != nil {
					state.fail("pull", lastError)
				} else {
					state.fail("pull", errNoDevice)
				}
				break
			}

			potentialDevices = removeDevice(potentialDevices, selected)

			// Fetch the block, while marking the selected device as in use so that
			// leastBusy can select another device when someone else asks.
			activity.using(selected)
			buf, lastError := p.model.requestGlobal(selected, p.folder, state.file.Name, state.block.Offset, int(state.block.Size), state.block.Hash, 0, nil)
			activity.done(selected)
			if lastError != nil {
				continue
			}

			// Verify that the received block matches the desired hash, if not
			// try pulling it from another device.
			_, lastError = scanner.VerifyBuffer(buf, state.block)
			if lastError != nil {
				continue
			}

			// Save the block data we got from the cluster
			_, err = fd.WriteAt(buf, state.block.Offset)
			if err != nil {
				state.fail("save", err)
			} else {
				state.pullDone()
			}
			break
		}
		out <- state.sharedPullerState
	}
}

func (p *rwFolder) performFinish(state *sharedPullerState) {
	var err error
	defer func() {
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"folder": p.folder,
			"item":   state.file.Name,
			"error":  err,
		})
	}()

	// Set the correct permission bits on the new file
	if !p.ignorePerms {
		err = os.Chmod(state.tempName, os.FileMode(state.file.Flags&0777))
		if err != nil {
			l.Warnln("Puller: final:", err)
			return
		}
	}

	// Set the correct timestamp on the new file
	t := time.Unix(state.file.Modified, 0)
	err = os.Chtimes(state.tempName, t, t)
	if err != nil {
		if p.lenientMtimes {
			// We accept the failure with a warning here and allow the sync to
			// continue. We'll sync the new mtime back to the other devices later.
			// If they have the same problem & setting, we might never get in
			// sync.
			l.Infof("Puller (folder %q, file %q): final: %v (continuing anyway as requested)", p.folder, state.file.Name, err)
		} else {
			l.Warnln("Puller: final:", err)
			return
		}
	}

	if p.inConflict(state.version, state.file.Version) {
		// The new file has been changed in conflict with the existing one. We
		// should file it away as a conflict instead of just removing or
		// archiving. Also merge with the version vector we had, to indicate
		// we have resolved the conflict.
		state.file.Version = state.file.Version.Merge(state.version)
		err = osutil.InWritableDir(moveForConflict, state.realName)
	} else if p.versioner != nil {
		// If we should use versioning, let the versioner archive the old
		// file before we replace it. Archiving a non-existent file is not
		// an error.
		err = p.versioner.Archive(state.realName)
	} else {
		err = nil
	}
	if err != nil {
		l.Warnln("Puller: final:", err)
		return
	}

	// If the target path is a symlink or a directory, we cannot copy
	// over it, hence remove it before proceeding.
	stat, err := osutil.Lstat(state.realName)
	if err == nil && (stat.IsDir() || stat.Mode()&os.ModeSymlink != 0) {
		osutil.InWritableDir(os.Remove, state.realName)
	}
	// Replace the original content with the new one
	err = osutil.Rename(state.tempName, state.realName)
	if err != nil {
		l.Warnln("Puller: final:", err)
		return
	}

	// If it's a symlink, the target of the symlink is inside the file.
	if state.file.IsSymlink() {
		content, err := ioutil.ReadFile(state.realName)
		if err != nil {
			l.Warnln("Puller: final: reading symlink:", err)
			return
		}

		// Remove the file, and replace it with a symlink.
		err = osutil.InWritableDir(func(path string) error {
			os.Remove(path)
			return symlinks.Create(path, string(content), state.file.Flags)
		}, state.realName)
		if err != nil {
			l.Warnln("Puller: final: creating symlink:", err)
			return
		}
	}

	// Record the updated file in the index
	p.dbUpdates <- state.file
}

func (p *rwFolder) finisherRoutine(in <-chan *sharedPullerState) {
	for state := range in {
		if closed, err := state.finalClose(); closed {
			if debug {
				l.Debugln(p, "closing", state.file.Name)
			}
			if err != nil {
				l.Warnln("Puller: final:", err)
				continue
			}

			p.queue.Done(state.file.Name)
			if state.failed() == nil {
				p.performFinish(state)
			} else {
				events.Default.Log(events.ItemFinished, map[string]interface{}{
					"folder": p.folder,
					"item":   state.file.Name,
					"error":  state.failed(),
				})
			}
			p.model.receivedFile(p.folder, state.file.Name)
			if p.progressEmitter != nil {
				p.progressEmitter.Deregister(state)
			}
		}
	}
}

// Moves the given filename to the front of the job queue
func (p *rwFolder) BringToFront(filename string) {
	p.queue.BringToFront(filename)
}

func (p *rwFolder) Jobs() ([]string, []string) {
	return p.queue.Jobs()
}

// dbUpdaterRoutine aggregates db updates and commits them in batches no
// larger than 1000 items, and no more delayed than 2 seconds.
func (p *rwFolder) dbUpdaterRoutine() {
	const (
		maxBatchSize = 1000
		maxBatchTime = 2 * time.Second
	)

	batch := make([]protocol.FileInfo, 0, maxBatchSize)
	tick := time.NewTicker(maxBatchTime)
	defer tick.Stop()

loop:
	for {
		select {
		case file, ok := <-p.dbUpdates:
			if !ok {
				break loop
			}

			file.LocalVersion = 0
			batch = append(batch, file)

			if len(batch) == maxBatchSize {
				p.model.updateLocals(p.folder, batch)
				batch = batch[:0]
			}

		case <-tick.C:
			if len(batch) > 0 {
				p.model.updateLocals(p.folder, batch)
				batch = batch[:0]
			}
		}
	}

	if len(batch) > 0 {
		p.model.updateLocals(p.folder, batch)
	}
}

func (p *rwFolder) inConflict(current, replacement protocol.Vector) bool {
	if current.Concurrent(replacement) {
		// Obvious case
		return true
	}
	if replacement.Counter(p.shortID) > current.Counter(p.shortID) {
		// The replacement file contains a higher version for ourselves than
		// what we have. This isn't supposed to be possible, since it's only
		// we who can increment that counter. We take it as a sign that
		// something is wrong (our index may have been corrupted or removed)
		// and flag it as a conflict.
		return true
	}
	return false
}

func invalidateFolder(cfg *config.Configuration, folderID string, err error) {
	for i := range cfg.Folders {
		folder := &cfg.Folders[i]
		if folder.ID == folderID {
			folder.Invalid = err.Error()
			return
		}
	}
}

func removeDevice(devices []protocol.DeviceID, device protocol.DeviceID) []protocol.DeviceID {
	for i := range devices {
		if devices[i] == device {
			devices[i] = devices[len(devices)-1]
			return devices[:len(devices)-1]
		}
	}
	return devices
}

func moveForConflict(name string) error {
	ext := filepath.Ext(name)
	withoutExt := name[:len(name)-len(ext)]
	newName := withoutExt + time.Now().Format(".sync-conflict-20060102-150405") + ext
	return os.Rename(name, newName)
}

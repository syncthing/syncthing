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
	"sort"
	"time"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/scanner"
	"github.com/syncthing/syncthing/lib/symlinks"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/versioner"
)

// TODO: Stop on errors

const (
	pauseIntv     = 60 * time.Second
	nextPullIntv  = 10 * time.Second
	shortPullIntv = time.Second
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

// Which filemode bits to preserve
const retainBits = os.ModeSetgid | os.ModeSetuid | os.ModeSticky

var (
	activity    = newDeviceActivity()
	errNoDevice = errors.New("no available source device")
)

const (
	dbUpdateHandleDir = iota
	dbUpdateDeleteDir
	dbUpdateHandleFile
	dbUpdateDeleteFile
	dbUpdateShortcutFile
)

const (
	defaultCopiers = 1
	defaultPullers = 16
)

type dbUpdateJob struct {
	file    protocol.FileInfo
	jobType int
}

type rwFolder struct {
	stateTracker

	model            *Model
	progressEmitter  *ProgressEmitter
	virtualMtimeRepo *db.VirtualMtimeRepo

	folder      string
	dir         string
	scanIntv    time.Duration
	versioner   versioner.Versioner
	ignorePerms bool
	copiers     int
	pullers     int
	shortID     uint64
	order       config.PullOrder

	stop        chan struct{}
	queue       *jobQueue
	dbUpdates   chan dbUpdateJob
	scanTimer   *time.Timer
	pullTimer   *time.Timer
	delayScan   chan time.Duration
	scanNow     chan rescanRequest
	remoteIndex chan struct{} // An index update was received, we should re-evaluate needs

	errors    map[string]string // path -> error string
	errorsMut sync.Mutex
}

func newRWFolder(m *Model, shortID uint64, cfg config.FolderConfiguration) *rwFolder {
	p := &rwFolder{
		stateTracker: stateTracker{
			folder: cfg.ID,
			mut:    sync.NewMutex(),
		},

		model:            m,
		progressEmitter:  m.progressEmitter,
		virtualMtimeRepo: db.NewVirtualMtimeRepo(m.db, cfg.ID),

		folder:      cfg.ID,
		dir:         cfg.Path(),
		scanIntv:    time.Duration(cfg.RescanIntervalS) * time.Second,
		ignorePerms: cfg.IgnorePerms,
		copiers:     cfg.Copiers,
		pullers:     cfg.Pullers,
		shortID:     shortID,
		order:       cfg.Order,

		stop:        make(chan struct{}),
		queue:       newJobQueue(),
		pullTimer:   time.NewTimer(shortPullIntv),
		scanTimer:   time.NewTimer(time.Millisecond), // The first scan should be done immediately.
		delayScan:   make(chan time.Duration),
		scanNow:     make(chan rescanRequest),
		remoteIndex: make(chan struct{}, 1), // This needs to be 1-buffered so that we queue a notification if we're busy doing a pull when it comes.

		errorsMut: sync.NewMutex(),
	}

	if p.copiers == 0 {
		p.copiers = defaultCopiers
	}
	if p.pullers == 0 {
		p.pullers = defaultPullers
	}

	return p
}

// Helper function to check whether either the ignorePerm flag has been
// set on the local host or the FlagNoPermBits has been set on the file/dir
// which is being pulled.
func (p *rwFolder) ignorePermissions(file protocol.FileInfo) bool {
	return p.ignorePerms || file.Flags&protocol.FlagNoPermBits != 0
}

// Serve will run scans and pulls. It will return when Stop()ed or on a
// critical error.
func (p *rwFolder) Serve() {
	if debug {
		l.Debugln(p, "starting")
		defer l.Debugln(p, "exiting")
	}

	defer func() {
		p.pullTimer.Stop()
		p.scanTimer.Stop()
		// TODO: Should there be an actual FolderStopped state?
		p.setState(FolderIdle)
	}()

	var prevVer int64
	var prevIgnoreHash string

	rescheduleScan := func() {
		if p.scanIntv == 0 {
			// We should not run scans, so it should not be rescheduled.
			return
		}

		// Sleep a random time between 3/4 and 5/4 of the configured interval.
		sleepNanos := (p.scanIntv.Nanoseconds()*3 + rand.Int63n(2*p.scanIntv.Nanoseconds())) / 4
		intv := time.Duration(sleepNanos) * time.Nanosecond

		if debug {
			l.Debugln(p, "next rescan in", intv)
		}
		p.scanTimer.Reset(intv)
	}

	// We don't start pulling files until a scan has been completed.
	initialScanCompleted := false

	for {
		select {
		case <-p.stop:
			return

		case <-p.remoteIndex:
			prevVer = 0
			p.pullTimer.Reset(shortPullIntv)
			if debug {
				l.Debugln(p, "remote index updated, rescheduling pull")
			}

		case <-p.pullTimer.C:
			if !initialScanCompleted {
				if debug {
					l.Debugln(p, "skip (initial)")
				}
				p.pullTimer.Reset(nextPullIntv)
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
			curVer, ok := p.model.RemoteLocalVersion(p.folder)
			if !ok || curVer == prevVer {
				if debug {
					l.Debugln(p, "skip (curVer == prevVer)", prevVer, ok)
				}
				p.pullTimer.Reset(nextPullIntv)
				continue
			}

			if debug {
				l.Debugln(p, "pulling", prevVer, curVer)
			}

			p.setState(FolderSyncing)
			p.clearErrors()
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

					if lv, ok := p.model.RemoteLocalVersion(p.folder); ok && lv < curVer {
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
					p.pullTimer.Reset(nextPullIntv)
					break
				}

				if tries > 10 {
					// We've tried a bunch of times to get in sync, but
					// we're not making it. Probably there are write
					// errors preventing us. Flag this with a warning and
					// wait a bit longer before retrying.
					l.Infof("Folder %q isn't making progress. Pausing puller for %v.", p.folder, pauseIntv)
					if debug {
						l.Debugln(p, "next pull in", pauseIntv)
					}

					if folderErrors := p.currentErrors(); len(folderErrors) > 0 {
						events.Default.Log(events.FolderErrors, map[string]interface{}{
							"folder": p.folder,
							"errors": folderErrors,
						})
					}

					p.pullTimer.Reset(pauseIntv)
					break
				}
			}
			p.setState(FolderIdle)

		// The reason for running the scanner from within the puller is that
		// this is the easiest way to make sure we are not doing both at the
		// same time.
		case <-p.scanTimer.C:
			if err := p.model.CheckFolderHealth(p.folder); err != nil {
				l.Infoln("Skipping folder", p.folder, "scan due to folder error:", err)
				rescheduleScan()
				continue
			}

			if debug {
				l.Debugln(p, "rescan")
			}

			if err := p.model.internalScanFolderSubs(p.folder, nil); err != nil {
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

		case req := <-p.scanNow:
			if err := p.model.CheckFolderHealth(p.folder); err != nil {
				l.Infoln("Skipping folder", p.folder, "scan due to folder error:", err)
				req.err <- err
				continue
			}

			if debug {
				l.Debugln(p, "forced rescan")
			}

			if err := p.model.internalScanFolderSubs(p.folder, req.subs); err != nil {
				// Potentially sets the error twice, once in the scanner just
				// by doing a check, and once here, if the error returned is
				// the same one as returned by CheckFolderHealth, though
				// duplicate set is handled by setError.
				p.setError(err)
				req.err <- err
				continue
			}

			req.err <- nil

		case next := <-p.delayScan:
			p.scanTimer.Reset(next)
		}
	}
}

func (p *rwFolder) Stop() {
	close(p.stop)
}

func (p *rwFolder) IndexUpdated() {
	select {
	case p.remoteIndex <- struct{}{}:
	default:
		// We might be busy doing a pull and thus not reading from this
		// channel. The channel is 1-buffered, so one notification will be
		// queued to ensure we recheck after the pull, but beyond that we must
		// make sure to not block index receiving.
	}
}

func (p *rwFolder) Scan(subs []string) error {
	req := rescanRequest{
		subs: subs,
		err:  make(chan error),
	}
	p.scanNow <- req
	return <-req.err
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

	updateWg := sync.NewWaitGroup()
	copyWg := sync.NewWaitGroup()
	pullWg := sync.NewWaitGroup()
	doneWg := sync.NewWaitGroup()

	if debug {
		l.Debugln(p, "c", p.copiers, "p", p.pullers)
	}

	p.dbUpdates = make(chan dbUpdateJob)
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
	pullFileSize := int64(0)

	fileDeletions := map[string]protocol.FileInfo{}
	dirDeletions := []protocol.FileInfo{}
	buckets := map[string][]protocol.FileInfo{}

	folderFiles.WithNeed(protocol.LocalDeviceID, func(intf db.FileIntf) bool {
		// Needed items are delivered sorted lexicographically. We'll handle
		// directories as they come along, so parents before children. Files
		// are queued and the order may be changed later.

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
			pullFileSize += file.Size()
			p.queue.Push(file.Name, file.Size(), file.Modified)
		}

		changed++
		return true
	})

	// Check if we are able to store all files on disk
	if pullFileSize > 0 {
		folder, ok := p.model.cfg.Folders()[p.folder]
		if ok {
			if free, err := osutil.DiskFreeBytes(folder.Path()); err == nil && free < pullFileSize {
				l.Infof("Puller (folder %q): insufficient disk space available to pull %d files (%.2fMB)", p.folder, changed, float64(pullFileSize)/1024/1024)
				return 0
			}
		}
	}

	// Reorder the file queue according to configuration

	switch p.order {
	case config.OrderRandom:
		p.queue.Shuffle()
	case config.OrderAlphabetic:
		// The queue is already in alphabetic order.
	case config.OrderSmallestFirst:
		p.queue.SortSmallestFirst()
	case config.OrderLargestFirst:
		p.queue.SortLargestFirst()
	case config.OrderOldestFirst:
		p.queue.SortOldestFirst()
	case config.OrderNewestFirst:
		p.queue.SortNewestFirst()
	}

	// Process the file queue

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
	events.Default.Log(events.ItemStarted, map[string]string{
		"folder": p.folder,
		"item":   file.Name,
		"type":   "dir",
		"action": "update",
	})

	defer func() {
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"folder": p.folder,
			"item":   file.Name,
			"error":  events.Error(err),
			"type":   "dir",
			"action": "update",
		})
	}()

	realName := filepath.Join(p.dir, file.Name)
	mode := os.FileMode(file.Flags & 0777)
	if p.ignorePermissions(file) {
		mode = 0777
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
		err = osutil.InWritableDir(osutil.Remove, realName)
		if err != nil {
			l.Infof("Puller (folder %q, dir %q): %v", p.folder, file.Name, err)
			p.newError(file.Name, err)
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
			if err != nil || p.ignorePermissions(file) {
				return err
			}

			// Stat the directory so we can check its permissions.
			info, err := osutil.Lstat(path)
			if err != nil {
				return err
			}

			// Mask for the bits we want to preserve and add them in to the
			// directories permissions.
			return os.Chmod(path, mode|(info.Mode()&retainBits))
		}

		if err = osutil.InWritableDir(mkdir, realName); err == nil {
			p.dbUpdates <- dbUpdateJob{file, dbUpdateHandleDir}
		} else {
			l.Infof("Puller (folder %q, dir %q): %v", p.folder, file.Name, err)
			p.newError(file.Name, err)
		}
		return
	// Weird error when stat()'ing the dir. Probably won't work to do
	// anything else with it if we can't even stat() it.
	case err != nil:
		l.Infof("Puller (folder %q, dir %q): %v", p.folder, file.Name, err)
		p.newError(file.Name, err)
		return
	}

	// The directory already exists, so we just correct the mode bits. (We
	// don't handle modification times on directories, because that sucks...)
	// It's OK to change mode bits on stuff within non-writable directories.
	if p.ignorePermissions(file) {
		p.dbUpdates <- dbUpdateJob{file, dbUpdateHandleDir}
	} else if err := os.Chmod(realName, mode|(info.Mode()&retainBits)); err == nil {
		p.dbUpdates <- dbUpdateJob{file, dbUpdateHandleDir}
	} else {
		l.Infof("Puller (folder %q, dir %q): %v", p.folder, file.Name, err)
		p.newError(file.Name, err)
	}
}

// deleteDir attempts to delete the given directory
func (p *rwFolder) deleteDir(file protocol.FileInfo) {
	var err error
	events.Default.Log(events.ItemStarted, map[string]string{
		"folder": p.folder,
		"item":   file.Name,
		"type":   "dir",
		"action": "delete",
	})
	defer func() {
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"folder": p.folder,
			"item":   file.Name,
			"error":  events.Error(err),
			"type":   "dir",
			"action": "delete",
		})
	}()

	realName := filepath.Join(p.dir, file.Name)
	// Delete any temporary files lying around in the directory
	dir, _ := os.Open(realName)
	if dir != nil {
		files, _ := dir.Readdirnames(-1)
		for _, file := range files {
			if defTempNamer.IsTemporary(file) {
				osutil.InWritableDir(osutil.Remove, filepath.Join(realName, file))
			}
		}
	}

	err = osutil.InWritableDir(osutil.Remove, realName)
	if err == nil || os.IsNotExist(err) {
		// It was removed or it doesn't exist to start with
		p.dbUpdates <- dbUpdateJob{file, dbUpdateDeleteDir}
	} else if _, serr := os.Lstat(realName); serr != nil && !os.IsPermission(serr) {
		// We get an error just looking at the directory, and it's not a
		// permission problem. Lets assume the error is in fact some variant
		// of "file does not exist" (possibly expressed as some parent being a
		// file and not a directory etc) and that the delete is handled.
		p.dbUpdates <- dbUpdateJob{file, dbUpdateDeleteDir}
	} else {
		l.Infof("Puller (folder %q, dir %q): delete: %v", p.folder, file.Name, err)
		p.newError(file.Name, err)
	}
}

// deleteFile attempts to delete the given file
func (p *rwFolder) deleteFile(file protocol.FileInfo) {
	var err error
	events.Default.Log(events.ItemStarted, map[string]string{
		"folder": p.folder,
		"item":   file.Name,
		"type":   "file",
		"action": "delete",
	})
	defer func() {
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"folder": p.folder,
			"item":   file.Name,
			"error":  events.Error(err),
			"type":   "file",
			"action": "delete",
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
		err = osutil.InWritableDir(osutil.Remove, realName)
	}

	if err == nil || os.IsNotExist(err) {
		// It was removed or it doesn't exist to start with
		p.dbUpdates <- dbUpdateJob{file, dbUpdateDeleteFile}
	} else if _, serr := os.Lstat(realName); serr != nil && !os.IsPermission(serr) {
		// We get an error just looking at the file, and it's not a permission
		// problem. Lets assume the error is in fact some variant of "file
		// does not exist" (possibly expressed as some parent being a file and
		// not a directory etc) and that the delete is handled.
		p.dbUpdates <- dbUpdateJob{file, dbUpdateDeleteFile}
	} else {
		l.Infof("Puller (folder %q, file %q): delete: %v", p.folder, file.Name, err)
		p.newError(file.Name, err)
	}
}

// renameFile attempts to rename an existing file to a destination
// and set the right attributes on it.
func (p *rwFolder) renameFile(source, target protocol.FileInfo) {
	var err error
	events.Default.Log(events.ItemStarted, map[string]string{
		"folder": p.folder,
		"item":   source.Name,
		"type":   "file",
		"action": "delete",
	})
	events.Default.Log(events.ItemStarted, map[string]string{
		"folder": p.folder,
		"item":   target.Name,
		"type":   "file",
		"action": "update",
	})
	defer func() {
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"folder": p.folder,
			"item":   source.Name,
			"error":  events.Error(err),
			"type":   "file",
			"action": "delete",
		})
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"folder": p.folder,
			"item":   target.Name,
			"error":  events.Error(err),
			"type":   "file",
			"action": "update",
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

		p.dbUpdates <- dbUpdateJob{source, dbUpdateDeleteFile}

		err = p.shortcutFile(target)
		if err != nil {
			l.Infof("Puller (folder %q, file %q): rename from %q metadata: %v", p.folder, target.Name, source.Name, err)
			p.newError(target.Name, err)
			return
		}

		p.dbUpdates <- dbUpdateJob{target, dbUpdateHandleFile}
	} else {
		// We failed the rename so we have a source file that we still need to
		// get rid of. Attempt to delete it instead so that we make *some*
		// progress. The target is unhandled.

		err = osutil.InWritableDir(osutil.Remove, from)
		if err != nil {
			l.Infof("Puller (folder %q, file %q): delete %q after failed rename: %v", p.folder, target.Name, source.Name, err)
			p.newError(target.Name, err)
			return
		}

		p.dbUpdates <- dbUpdateJob{source, dbUpdateDeleteFile}
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
func (p *rwFolder) handleFile(file protocol.FileInfo, copyChan chan<- copyBlocksState, finisherChan chan<- *sharedPullerState) {
	curFile, ok := p.model.CurrentFolderFile(p.folder, file.Name)

	if ok && len(curFile.Blocks) == len(file.Blocks) && scanner.BlocksEqual(curFile.Blocks, file.Blocks) {
		// We are supposed to copy the entire file, and then fetch nothing. We
		// are only updating metadata, so we don't actually *need* to make the
		// copy.
		if debug {
			l.Debugln(p, "taking shortcut on", file.Name)
		}

		events.Default.Log(events.ItemStarted, map[string]string{
			"folder": p.folder,
			"item":   file.Name,
			"type":   "file",
			"action": "metadata",
		})

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
			"error":  events.Error(err),
			"type":   "file",
			"action": "metadata",
		})

		if err != nil {
			l.Infoln("Puller: shortcut:", err)
			p.newError(file.Name, err)
		} else {
			p.dbUpdates <- dbUpdateJob{file, dbUpdateShortcutFile}
		}

		return
	}

	events.Default.Log(events.ItemStarted, map[string]string{
		"folder": p.folder,
		"item":   file.Name,
		"type":   "file",
		"action": "update",
	})

	scanner.PopulateOffsets(file.Blocks)

	// Figure out the absolute filenames we need once and for all
	tempName := filepath.Join(p.dir, defTempNamer.TempName(file.Name))
	realName := filepath.Join(p.dir, file.Name)

	reused := 0
	var blocks []protocol.BlockInfo

	// Check for an old temporary file which might have some blocks we could
	// reuse.
	tempBlocks, err := scanner.HashFile(tempName, protocol.BlockSize, 0, nil)
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
			// sharedpuller not to panic when it fails to exclusively create a
			// file which already exists
			osutil.InWritableDir(osutil.Remove, tempName)
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
		ignorePerms: p.ignorePermissions(file),
		version:     curFile.Version,
		mut:         sync.NewMutex(),
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
func (p *rwFolder) shortcutFile(file protocol.FileInfo) error {
	realName := filepath.Join(p.dir, file.Name)
	if !p.ignorePermissions(file) {
		if err := os.Chmod(realName, os.FileMode(file.Flags&0777)); err != nil {
			l.Infof("Puller (folder %q, file %q): shortcut: chmod: %v", p.folder, file.Name, err)
			p.newError(file.Name, err)
			return err
		}
	}

	t := time.Unix(file.Modified, 0)
	if err := os.Chtimes(realName, t, t); err != nil {
		// Try using virtual mtimes
		info, err := os.Stat(realName)
		if err != nil {
			l.Infof("Puller (folder %q, file %q): shortcut: unable to stat file: %v", p.folder, file.Name, err)
			p.newError(file.Name, err)
			return err
		}

		p.virtualMtimeRepo.UpdateMtime(file.Name, info.ModTime(), t)
	}

	// This may have been a conflict. We should merge the version vectors so
	// that our clock doesn't move backwards.
	if cur, ok := p.model.CurrentFolderFile(p.folder, file.Name); ok {
		file.Version = file.Version.Merge(cur.Version)
	}

	return nil
}

// shortcutSymlink changes the symlinks type if necessary.
func (p *rwFolder) shortcutSymlink(file protocol.FileInfo) (err error) {
	err = symlinks.ChangeType(filepath.Join(p.dir, file.Name), file.Flags)
	if err != nil {
		l.Infof("Puller (folder %q, file %q): symlink shortcut: %v", p.folder, file.Name, err)
		p.newError(file.Name, err)
	}
	return
}

// copierRoutine reads copierStates until the in channel closes and performs
// the relevant copies when possible, or passes it to the puller routine.
func (p *rwFolder) copierRoutine(in <-chan copyBlocksState, pullChan chan<- pullBlockState, out chan<- *sharedPullerState) {
	buf := make([]byte, protocol.BlockSize)

	for state := range in {
		dstFd, err := state.tempFile()
		if err != nil {
			// Nothing more to do for this failed file, since we couldn't create a temporary for it.
			out <- state.sharedPullerState
			continue
		}

		if p.progressEmitter != nil {
			p.progressEmitter.Register(state.sharedPullerState)
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
			out <- state.sharedPullerState
			continue
		}

		// Get an fd to the temporary file. Technically we don't need it until
		// after fetching the block, but if we run into an error here there is
		// no point in issuing the request to the network.
		fd, err := state.tempFile()
		if err != nil {
			out <- state.sharedPullerState
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
				if debug {
					l.Debugln("request:", p.folder, state.file.Name, state.block.Offset, state.block.Size, "returned error:", lastError)
				}
				continue
			}

			// Verify that the received block matches the desired hash, if not
			// try pulling it from another device.
			_, lastError = scanner.VerifyBuffer(buf, state.block)
			if lastError != nil {
				if debug {
					l.Debugln("request:", p.folder, state.file.Name, state.block.Offset, state.block.Size, "hash mismatch")
				}
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

func (p *rwFolder) performFinish(state *sharedPullerState) error {
	// Set the correct permission bits on the new file
	if !p.ignorePermissions(state.file) {
		if err := os.Chmod(state.tempName, os.FileMode(state.file.Flags&0777)); err != nil {
			return err
		}
	}

	// Set the correct timestamp on the new file
	t := time.Unix(state.file.Modified, 0)
	if err := os.Chtimes(state.tempName, t, t); err != nil {
		// Try using virtual mtimes instead
		info, err := os.Stat(state.tempName)
		if err != nil {
			return err
		}
		p.virtualMtimeRepo.UpdateMtime(state.file.Name, info.ModTime(), t)
	}

	if stat, err := osutil.Lstat(state.realName); err == nil {
		// There is an old file or directory already in place. We need to
		// handle that.

		switch {
		case stat.IsDir() || stat.Mode()&os.ModeSymlink != 0:
			// It's a directory or a symlink. These are not versioned or
			// archived for conflicts, only removed (which of course fails for
			// non-empty directories).

			// TODO: This is the place where we want to remove temporary files
			// and future hard ignores before attempting a directory delete.
			// Should share code with p.deletDir().

			if err = osutil.InWritableDir(osutil.Remove, state.realName); err != nil {
				return err
			}

		case p.inConflict(state.version, state.file.Version):
			// The new file has been changed in conflict with the existing one. We
			// should file it away as a conflict instead of just removing or
			// archiving. Also merge with the version vector we had, to indicate
			// we have resolved the conflict.

			state.file.Version = state.file.Version.Merge(state.version)
			if err = osutil.InWritableDir(moveForConflict, state.realName); err != nil {
				return err
			}

		case p.versioner != nil:
			// If we should use versioning, let the versioner archive the old
			// file before we replace it. Archiving a non-existent file is not
			// an error.

			if err = p.versioner.Archive(state.realName); err != nil {
				return err
			}
		}
	}

	// Replace the original content with the new one
	if err := osutil.Rename(state.tempName, state.realName); err != nil {
		return err
	}

	// If it's a symlink, the target of the symlink is inside the file.
	if state.file.IsSymlink() {
		content, err := ioutil.ReadFile(state.realName)
		if err != nil {
			return err
		}

		// Remove the file, and replace it with a symlink.
		err = osutil.InWritableDir(func(path string) error {
			os.Remove(path)
			return symlinks.Create(path, string(content), state.file.Flags)
		}, state.realName)
		if err != nil {
			return err
		}
	}

	// Record the updated file in the index
	p.dbUpdates <- dbUpdateJob{state.file, dbUpdateHandleFile}
	return nil
}

func (p *rwFolder) finisherRoutine(in <-chan *sharedPullerState) {
	for state := range in {
		if closed, err := state.finalClose(); closed {
			if debug {
				l.Debugln(p, "closing", state.file.Name)
			}

			p.queue.Done(state.file.Name)

			if err == nil {
				err = p.performFinish(state)
			}

			if err != nil {
				l.Infoln("Puller: final:", err)
				p.newError(state.file.Name, err)
			}
			events.Default.Log(events.ItemFinished, map[string]interface{}{
				"folder": p.folder,
				"item":   state.file.Name,
				"error":  events.Error(err),
				"type":   "file",
				"action": "update",
			})

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

func (p *rwFolder) DelayScan(next time.Duration) {
	p.delayScan <- next
}

// dbUpdaterRoutine aggregates db updates and commits them in batches no
// larger than 1000 items, and no more delayed than 2 seconds.
func (p *rwFolder) dbUpdaterRoutine() {
	const (
		maxBatchSize = 1000
		maxBatchTime = 2 * time.Second
	)

	batch := make([]dbUpdateJob, 0, maxBatchSize)
	files := make([]protocol.FileInfo, 0, maxBatchSize)
	tick := time.NewTicker(maxBatchTime)
	defer tick.Stop()

	handleBatch := func() {
		found := false
		var lastFile protocol.FileInfo

		for _, job := range batch {
			files = append(files, job.file)
			if job.file.IsInvalid() || (job.file.IsDirectory() && !job.file.IsSymlink()) {
				continue
			}

			if job.jobType&(dbUpdateHandleFile|dbUpdateDeleteFile) == 0 {
				continue
			}

			found = true
			lastFile = job.file
		}

		p.model.updateLocals(p.folder, files)

		if found {
			p.model.receivedFile(p.folder, lastFile)
		}

		batch = batch[:0]
		files = files[:0]
	}

loop:
	for {
		select {
		case job, ok := <-p.dbUpdates:
			if !ok {
				break loop
			}

			job.file.LocalVersion = 0
			batch = append(batch, job)

			if len(batch) == maxBatchSize {
				handleBatch()
			}

		case <-tick.C:
			if len(batch) > 0 {
				handleBatch()
			}
		}
	}

	if len(batch) > 0 {
		handleBatch()
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
	err := os.Rename(name, newName)
	if os.IsNotExist(err) {
		// We were supposed to move a file away but it does not exist. Either
		// the user has already moved it away, or the conflict was between a
		// remote modification and a local delete. In either way it does not
		// matter, go ahead as if the move succeeded.
		return nil
	}
	return err
}

func (p *rwFolder) newError(path string, err error) {
	p.errorsMut.Lock()
	defer p.errorsMut.Unlock()

	// We might get more than one error report for a file (i.e. error on
	// Write() followed by Close()); we keep the first error as that is
	// probably closer to the root cause.
	if _, ok := p.errors[path]; ok {
		return
	}

	p.errors[path] = err.Error()
}

func (p *rwFolder) clearErrors() {
	p.errorsMut.Lock()
	p.errors = make(map[string]string)
	p.errorsMut.Unlock()
}

func (p *rwFolder) currentErrors() []fileError {
	p.errorsMut.Lock()
	errors := make([]fileError, 0, len(p.errors))
	for path, err := range p.errors {
		errors = append(errors, fileError{path, err})
	}
	sort.Sort(fileErrorList(errors))
	p.errorsMut.Unlock()
	return errors
}

// A []fileError is sent as part of an event and will be JSON serialized.
type fileError struct {
	Path string `json:"path"`
	Err  string `json:"error"`
}

type fileErrorList []fileError

func (l fileErrorList) Len() int {
	return len(l)
}

func (l fileErrorList) Less(a, b int) bool {
	return l[a].Path < l[b].Path
}

func (l fileErrorList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}

// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"errors"
	"fmt"
	"math/rand"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/versioner"
	"github.com/syncthing/syncthing/lib/weakhash"
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
)

const (
	dbUpdateHandleDir = iota
	dbUpdateDeleteDir
	dbUpdateHandleFile
	dbUpdateDeleteFile
	dbUpdateShortcutFile
	dbUpdateHandleSymlink
)

const (
	defaultCopiers     = 2
	defaultPullers     = 64
	defaultPullerSleep = 10 * time.Second
	defaultPullerPause = 60 * time.Second
)

type dbUpdateJob struct {
	file    protocol.FileInfo
	jobType int
}

type sendReceiveFolder struct {
	folder

	fs        fs.Filesystem
	versioner versioner.Versioner
	sleep     time.Duration
	pause     time.Duration

	queue       *jobQueue
	dbUpdates   chan dbUpdateJob
	pullTimer   *time.Timer
	remoteIndex chan struct{} // An index update was received, we should re-evaluate needs

	errors    map[string]string // path -> error string
	errorsMut sync.Mutex

	blockStats    map[string]int
	blockStatsMut sync.Mutex
}

func newSendReceiveFolder(model *Model, cfg config.FolderConfiguration, ver versioner.Versioner, fs fs.Filesystem) service {
	f := &sendReceiveFolder{
		folder: newFolder(model, cfg),

		fs:        fs,
		versioner: ver,

		queue:       newJobQueue(),
		pullTimer:   time.NewTimer(time.Second),
		remoteIndex: make(chan struct{}, 1), // This needs to be 1-buffered so that we queue a notification if we're busy doing a pull when it comes.

		errorsMut: sync.NewMutex(),

		blockStats:    make(map[string]int),
		blockStatsMut: sync.NewMutex(),
	}

	f.configureCopiersAndPullers()

	return f
}

func (f *sendReceiveFolder) configureCopiersAndPullers() {
	if f.Copiers == 0 {
		f.Copiers = defaultCopiers
	}
	if f.Pullers == 0 {
		f.Pullers = defaultPullers
	}

	if f.PullerPauseS == 0 {
		f.pause = defaultPullerPause
	} else {
		f.pause = time.Duration(f.PullerPauseS) * time.Second
	}

	if f.PullerSleepS == 0 {
		f.sleep = defaultPullerSleep
	} else {
		f.sleep = time.Duration(f.PullerSleepS) * time.Second
	}
}

// Helper function to check whether either the ignorePerm flag has been
// set on the local host or the FlagNoPermBits has been set on the file/dir
// which is being pulled.
func (f *sendReceiveFolder) ignorePermissions(file protocol.FileInfo) bool {
	return f.IgnorePerms || file.NoPermissions
}

// Serve will run scans and pulls. It will return when Stop()ed or on a
// critical error.
func (f *sendReceiveFolder) Serve() {
	l.Debugln(f, "starting")
	defer l.Debugln(f, "exiting")

	defer func() {
		f.pullTimer.Stop()
		f.scan.timer.Stop()
		// TODO: Should there be an actual FolderStopped state?
		f.setState(FolderIdle)
	}()

	var prevSec int64
	var prevIgnoreHash string

	if f.FSWatcherEnabled {
		f.startWatcher()
	}

	for {
		select {
		case <-f.ctx.Done():
			return

		case <-f.remoteIndex:
			prevSec = 0
			f.pullTimer.Reset(0)
			l.Debugln(f, "remote index updated, rescheduling pull")

		case <-f.ignoresUpdated:
			if f.FSWatcherEnabled {
				f.restartWatcher()
			}
			f.IndexUpdated()

		case <-f.pullTimer.C:
			select {
			case <-f.initialScanFinished:
			default:
				// We don't start pulling files until a scan has been completed.
				l.Debugln(f, "skip (initial)")
				f.pullTimer.Reset(f.sleep)
				continue
			}

			f.model.fmut.RLock()
			curIgnores := f.model.folderIgnores[f.folderID]
			f.model.fmut.RUnlock()

			if newHash := curIgnores.Hash(); newHash != prevIgnoreHash {
				// The ignore patterns have changed. We need to re-evaluate if
				// there are files we need now that were ignored before.
				l.Debugln(f, "ignore patterns have changed, resetting prevVer")
				prevSec = 0
				prevIgnoreHash = newHash
			}

			// RemoteSequence() is a fast call, doesn't touch the database.
			curSeq, ok := f.model.RemoteSequence(f.folderID)
			if !ok || curSeq == prevSec {
				l.Debugln(f, "skip (curSeq == prevSeq)", prevSec, ok)
				f.pullTimer.Reset(f.sleep)
				continue
			}

			if err := f.model.CheckFolderHealth(f.folderID); err != nil {
				l.Infoln("Skipping pull of", f.Description(), "due to folder error:", err)
				f.pullTimer.Reset(f.sleep)
				continue
			}

			l.Debugln(f, "pulling", prevSec, curSeq)

			f.setState(FolderSyncing)
			f.clearErrors()
			tries := 0

			for {
				tries++

				changed := f.pullerIteration(curIgnores)
				l.Debugln(f, "changed", changed)

				if changed == 0 {
					// No files were changed by the puller, so we are in
					// sync. Remember the local version number and
					// schedule a resync a little bit into the future.

					if lv, ok := f.model.RemoteSequence(f.folderID); ok && lv < curSeq {
						// There's a corner case where the device we needed
						// files from disconnected during the puller
						// iteration. The files will have been removed from
						// the index, so we've concluded that we don't need
						// them, but at the same time we have the local
						// version that includes those files in curVer. So we
						// catch the case that sequence might have
						// decreased here.
						l.Debugln(f, "adjusting curVer", lv)
						curSeq = lv
					}
					prevSec = curSeq
					l.Debugln(f, "next pull in", f.sleep)
					f.pullTimer.Reset(f.sleep)
					break
				}

				if tries > 2 {
					// We've tried a bunch of times to get in sync, but
					// we're not making it. Probably there are write
					// errors preventing us. Flag this with a warning and
					// wait a bit longer before retrying.
					if folderErrors := f.currentErrors(); len(folderErrors) > 0 {
						events.Default.Log(events.FolderErrors, map[string]interface{}{
							"folder": f.folderID,
							"errors": folderErrors,
						})
					}

					l.Infof("Folder %v isn't making progress. Pausing puller for %v.", f.Description(), f.pause)
					l.Debugln(f, "next pull in", f.pause)

					f.pullTimer.Reset(f.pause)
					break
				}
			}
			f.setState(FolderIdle)

		// The reason for running the scanner from within the puller is that
		// this is the easiest way to make sure we are not doing both at the
		// same time.
		case <-f.scan.timer.C:
			l.Debugln(f, "Scanning subdirectories")
			f.scanTimerFired()

		case req := <-f.scan.now:
			req.err <- f.scanSubdirs(req.subdirs)

		case next := <-f.scan.delay:
			f.scan.timer.Reset(next)

		case fsEvents := <-f.watchChan:
			l.Debugln(f, "filesystem notification rescan")
			f.scanSubdirs(fsEvents)
		}
	}
}

func (f *sendReceiveFolder) IndexUpdated() {
	select {
	case f.remoteIndex <- struct{}{}:
	default:
		// We might be busy doing a pull and thus not reading from this
		// channel. The channel is 1-buffered, so one notification will be
		// queued to ensure we recheck after the pull, but beyond that we must
		// make sure to not block index receiving.
	}
}

func (f *sendReceiveFolder) String() string {
	return fmt.Sprintf("sendReceiveFolder/%s@%p", f.folderID, f)
}

// pullerIteration runs a single puller iteration for the given folder and
// returns the number items that should have been synced (even those that
// might have failed). One puller iteration handles all files currently
// flagged as needed in the folder.
func (f *sendReceiveFolder) pullerIteration(ignores *ignore.Matcher) int {
	pullChan := make(chan pullBlockState)
	copyChan := make(chan copyBlocksState)
	finisherChan := make(chan *sharedPullerState)

	updateWg := sync.NewWaitGroup()
	copyWg := sync.NewWaitGroup()
	pullWg := sync.NewWaitGroup()
	doneWg := sync.NewWaitGroup()

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

	f.model.fmut.RLock()
	folderFiles := f.model.folderFiles[f.folderID]
	f.model.fmut.RUnlock()

	changed := 0
	var processDirectly []protocol.FileInfo

	// Iterate the list of items that we need and sort them into piles.
	// Regular files to pull goes into the file queue, everything else
	// (directories, symlinks and deletes) goes into the "process directly"
	// pile.

	folderFiles.WithNeed(protocol.LocalDeviceID, func(intf db.FileIntf) bool {
		if shouldIgnore(intf, ignores, f.IgnoreDelete) {
			return true
		}

		if err := fileValid(intf); err != nil {
			// The file isn't valid so we can't process it. Pretend that we
			// tried and set the error for the file.
			f.newError("need", intf.FileName(), err)
			changed++
			return true
		}

		file := intf.(protocol.FileInfo)

		switch {
		case file.IsDeleted():
			processDirectly = append(processDirectly, file)
			changed++

		case file.Type == protocol.FileInfoTypeFile:
			// Queue files for processing after directories and symlinks, if
			// it has availability.

			devices := folderFiles.Availability(file.Name)
			for _, dev := range devices {
				if f.model.ConnectedTo(dev) {
					f.queue.Push(file.Name, file.Size, file.ModTime())
					changed++
					break
				}
			}

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
				if ok && !df.IsDeleted() && !df.IsSymlink() && !df.IsDirectory() {
					// Put files into buckets per first hash
					key := string(df.Blocks[0].Hash)
					buckets[key] = append(buckets[key], df)
				}
			}

		case fi.IsDirectory() && !fi.IsSymlink():
			l.Debugln("Handling directory", fi.Name)
			f.handleDir(fi)

		case fi.IsSymlink():
			l.Debugln("Handling symlink", fi.Name)
			f.handleSymlink(fi)

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

		// Verify that the thing we are handling lives inside a directory,
		// and not a symlink or empty space.
		if err := osutil.TraversesSymlink(f.fs, filepath.Dir(fi.Name)); err != nil {
			f.newError("traverses q", fi.Name, err)
			continue
		}

		// Check our list of files to be removed for a match, in which case
		// we can just do a rename instead.
		key := string(fi.Blocks[0].Hash)
		for i, candidate := range buckets[key] {
			if scanner.BlocksEqual(candidate.Blocks, fi.Blocks) {
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

				f.renameFile(desired, fi)

				f.queue.Done(fileName)
				continue nextFile
			}
		}

		// Handle the file normally, by coping and pulling, etc.
		f.handleFile(fi, copyChan, finisherChan)
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
		l.Debugln("Deleting file", file.Name)
		f.deleteFile(file)
	}

	for i := range dirDeletions {
		dir := dirDeletions[len(dirDeletions)-i-1]
		l.Debugln("Deleting dir", dir.Name)
		f.deleteDir(dir, ignores)
	}

	// Wait for db updates to complete
	close(f.dbUpdates)
	updateWg.Wait()

	return changed
}

// handleDir creates or updates the given directory
func (f *sendReceiveFolder) handleDir(file protocol.FileInfo) {
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
	if f.ignorePermissions(file) {
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
			if err != nil || f.ignorePermissions(file) {
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
			f.dbUpdates <- dbUpdateJob{file, dbUpdateHandleDir}
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
	if f.ignorePermissions(file) {
		f.dbUpdates <- dbUpdateJob{file, dbUpdateHandleDir}
	} else if err := f.fs.Chmod(file.Name, mode|(fs.FileMode(info.Mode())&retainBits)); err == nil {
		f.dbUpdates <- dbUpdateJob{file, dbUpdateHandleDir}
	} else {
		f.newError("dir chmod", file.Name, err)
	}
}

// handleSymlink creates or updates the given symlink
func (f *sendReceiveFolder) handleSymlink(file protocol.FileInfo) {
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
		f.dbUpdates <- dbUpdateJob{file, dbUpdateHandleSymlink}
	} else {
		f.newError("symlink create", file.Name, err)
	}
}

// deleteDir attempts to delete the given directory
func (f *sendReceiveFolder) deleteDir(file protocol.FileInfo, matcher *ignore.Matcher) {
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

	// Delete any temporary files lying around in the directory

	files, _ := f.fs.DirNames(file.Name)
	for _, dirFile := range files {
		fullDirFile := filepath.Join(file.Name, dirFile)
		if fs.IsTemporary(dirFile) || (matcher != nil && matcher.Match(fullDirFile).IsDeletable()) {
			f.fs.RemoveAll(fullDirFile)
		}
	}

	err = osutil.InWritableDir(f.fs.Remove, f.fs, file.Name)
	if err == nil || fs.IsNotExist(err) {
		// It was removed or it doesn't exist to start with
		f.dbUpdates <- dbUpdateJob{file, dbUpdateDeleteDir}
	} else if _, serr := f.fs.Lstat(file.Name); serr != nil && !fs.IsPermission(serr) {
		// We get an error just looking at the directory, and it's not a
		// permission problem. Lets assume the error is in fact some variant
		// of "file does not exist" (possibly expressed as some parent being a
		// file and not a directory etc) and that the delete is handled.
		f.dbUpdates <- dbUpdateJob{file, dbUpdateDeleteDir}
	} else {
		f.newError("delete dir", file.Name, err)
	}
}

// deleteFile attempts to delete the given file
func (f *sendReceiveFolder) deleteFile(file protocol.FileInfo) {
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
		f.dbUpdates <- dbUpdateJob{file, dbUpdateDeleteFile}
	} else if _, serr := f.fs.Lstat(file.Name); serr != nil && !fs.IsPermission(serr) {
		// We get an error just looking at the file, and it's not a permission
		// problem. Lets assume the error is in fact some variant of "file
		// does not exist" (possibly expressed as some parent being a file and
		// not a directory etc) and that the delete is handled.
		f.dbUpdates <- dbUpdateJob{file, dbUpdateDeleteFile}
	} else {
		f.newError("delete file", file.Name, err)
	}
}

// renameFile attempts to rename an existing file to a destination
// and set the right attributes on it.
func (f *sendReceiveFolder) renameFile(source, target protocol.FileInfo) {
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
		f.blockStatsMut.Lock()
		f.blockStats["total"] += len(target.Blocks)
		f.blockStats["renamed"] += len(target.Blocks)
		f.blockStatsMut.Unlock()

		// The file was renamed, so we have handled both the necessary delete
		// of the source and the creation of the target. Fix-up the metadata,
		// and update the local index of the target file.

		f.dbUpdates <- dbUpdateJob{source, dbUpdateDeleteFile}

		err = f.shortcutFile(target)
		if err != nil {
			err = fmt.Errorf("from %s: %s", source.Name, err.Error())
			f.newError("rename shortcut", target.Name, err)
			return
		}

		f.dbUpdates <- dbUpdateJob{target, dbUpdateHandleFile}
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

		f.dbUpdates <- dbUpdateJob{source, dbUpdateDeleteFile}
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
func (f *sendReceiveFolder) handleFile(file protocol.FileInfo, copyChan chan<- copyBlocksState, finisherChan chan<- *sharedPullerState) {
	curFile, hasCurFile := f.model.CurrentFolderFile(f.folderID, file.Name)

	have, need := scanner.BlockDiff(curFile.Blocks, file.Blocks)

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
			f.dbUpdates <- dbUpdateJob{file, dbUpdateShortcutFile}
		}

		return
	}

	tempName := fs.TempName(file.Name)

	scanner.PopulateOffsets(file.Blocks)

	var blocks []protocol.BlockInfo
	var blocksSize int64
	var reused []int32

	// Check for an old temporary file which might have some blocks we could
	// reuse.
	tempBlocks, err := scanner.HashFile(f.ctx, f.fs, tempName, protocol.BlockSize, nil, false)
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
		ignorePerms:      f.ignorePermissions(file),
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

// shortcutFile sets file mode and modification time, when that's the only
// thing that has changed.
func (f *sendReceiveFolder) shortcutFile(file protocol.FileInfo) error {
	if !f.ignorePermissions(file) {
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
	buf := make([]byte, protocol.BlockSize)

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

		if weakhash.Enabled {
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
						weakHashFinder, err = weakhash.NewFinder(file, protocol.BlockSize, hashesToFind)
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
		} else {
			l.Debugf("not weak hashing %s. weak hashing disabled", state.file.Name)
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
				continue
			}

			buf = buf[:int(block.Size)]

			found, err := weakHashFinder.Iterate(block.WeakHash, buf, func(offset int64) bool {
				if _, err := scanner.VerifyBuffer(buf, block); err != nil {
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

					_, err = fd.ReadAt(buf, protocol.BlockSize*int64(index))
					fd.Close()
					if err != nil {
						return false
					}

					hash, err := scanner.VerifyBuffer(buf, block)
					if err != nil {
						if hash != nil {
							l.Debugf("Finder block mismatch in %s:%s:%d expected %q got %q", folder, path, index, block.Hash, hash)
							err = f.model.finder.Fix(folder, path, index, block.Hash, hash)
							if err != nil {
								l.Warnln("finder fix:", err)
							}
						} else {
							l.Debugln("Finder failed to verify buffer", err)
						}
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

func (f *sendReceiveFolder) pullerRoutine(in <-chan pullBlockState, out chan<- *sharedPullerState) {
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

		if !f.DisableSparseFiles && state.reused == 0 && state.block.IsEmpty() {
			// There is no need to request a block of all zeroes. Pretend we
			// requested it and handled it correctly.
			state.pullDone(state.block)
			out <- state.sharedPullerState
			continue
		}

		var lastError error
		candidates := f.model.Availability(f.folderID, state.file.Name, state.file.Version, state.block)
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
			buf, lastError := f.model.requestGlobal(selected.ID, f.folderID, state.file.Name, state.block.Offset, int(state.block.Size), state.block.Hash, selected.FromTemporary)
			activity.done(selected)
			if lastError != nil {
				l.Debugln("request:", f.folderID, state.file.Name, state.block.Offset, state.block.Size, "returned error:", lastError)
				continue
			}

			// Verify that the received block matches the desired hash, if not
			// try pulling it from another device.
			_, lastError = scanner.VerifyBuffer(buf, state.block)
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
}

func (f *sendReceiveFolder) performFinish(state *sharedPullerState) error {
	// Set the correct permission bits on the new file
	if !f.ignorePermissions(state.file) {
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

		// Check that the file on disk is what we expect it to be according to
		// the database. If there's a mismatch here, there might be local
		// changes that we don't know about yet and we should scan before
		// touching the file.
		// There is also a case where we think the file should be there, but
		// it was removed, which is a conflict, yet creations always wins when
		// competing with a deletion, so no need to handle that specially.
		switch {
		// The file reappeared from nowhere, or mtime/size has changed, fallthrough -> rescan.
		case !state.hasCurFile || !stat.ModTime().Equal(state.curFile.ModTime()) || stat.Size() != state.curFile.Size:
			fallthrough
		// Permissions have changed, means the file has changed, rescan.
		case !f.ignorePermissions(state.curFile) && state.curFile.HasPermissionBits() && !scanner.PermsEqual(state.curFile.Permissions, curMode):
			l.Debugln("file modified but not rescanned; not finishing:", state.curFile.Name)
			// Scan() is synchronous (i.e. blocks until the scan is
			// completed and returns an error), but a scan can't happen
			// while we're in the puller routine. Request the scan in the
			// background and it'll be handled when the current pulling
			// sweep is complete. As we do retries, we'll queue the scan
			// for this file up to ten times, but the last nine of those
			// scans will be cheap...
			go f.Scan([]string{state.curFile.Name})
			return fmt.Errorf("file modified but not rescanned; will try again later")
		}

		switch {
		case stat.IsDir() || stat.IsSymlink():
			// It's a directory or a symlink. These are not versioned or
			// archived for conflicts, only removed (which of course fails for
			// non-empty directories).

			// TODO: This is the place where we want to remove temporary files
			// and future hard ignores before attempting a directory delete.
			// Should share code with f.deletDir().

			if err = osutil.InWritableDir(f.fs.Remove, f.fs, state.file.Name); err != nil {
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

			if err = f.versioner.Archive(state.file.Name); err != nil {
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
	f.dbUpdates <- dbUpdateJob{state.file, dbUpdateHandleFile}
	return nil
}

func (f *sendReceiveFolder) finisherRoutine(in <-chan *sharedPullerState) {
	for state := range in {
		if closed, err := state.finalClose(); closed {
			l.Debugln(f, "closing", state.file.Name)

			f.queue.Done(state.file.Name)

			if err == nil {
				err = f.performFinish(state)
			}

			if err != nil {
				f.newError("finisher", state.file.Name, err)
			} else {
				f.blockStatsMut.Lock()
				f.blockStats["total"] += state.reused + state.copyTotal + state.pullTotal
				f.blockStats["reused"] += state.reused
				f.blockStats["pulled"] += state.pullTotal
				f.blockStats["copyOrigin"] += state.copyOrigin
				f.blockStats["copyOriginShifted"] += state.copyOriginShifted
				f.blockStats["copyElsewhere"] += state.copyTotal - state.copyOrigin
				f.blockStatsMut.Unlock()
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

func (f *sendReceiveFolder) BlockStats() map[string]int {
	f.blockStatsMut.Lock()
	stats := make(map[string]int)
	for k, v := range f.blockStats {
		stats[k] = v
	}
	f.blockStatsMut.Unlock()
	return stats
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
func (f *sendReceiveFolder) dbUpdaterRoutine() {
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
			case dbUpdateHandleSymlink:
				// fsyncing symlinks is only supported by MacOS, ignore
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
		case job, ok := <-f.dbUpdates:
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

func (f *sendReceiveFolder) inConflict(current, replacement protocol.Vector) bool {
	if current.Concurrent(replacement) {
		// Obvious case
		return true
	}
	if replacement.Counter(f.model.shortID) > current.Counter(f.model.shortID) {
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
	l.Infof("Puller (folder %q, file %q): %s: %v", f.Description(), path, context, err)
	f.errors[path] = fmt.Sprintf("%s: %s", context, err.Error())
}

func (f *sendReceiveFolder) clearErrors() {
	f.errorsMut.Lock()
	f.errors = make(map[string]string)
	f.errorsMut.Unlock()
}

func (f *sendReceiveFolder) currentErrors() []fileError {
	f.errorsMut.Lock()
	errors := make([]fileError, 0, len(f.errors))
	for path, err := range f.errors {
		errors = append(errors, fileError{path, err})
	}
	sort.Sort(fileErrorList(errors))
	f.errorsMut.Unlock()
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

// fileValid returns nil when the file is valid for processing, or an error if it's not
func fileValid(file db.FileIntf) error {
	switch {
	case file.IsDeleted():
		// We don't care about file validity if we're not supposed to have it
		return nil

	case runtime.GOOS == "windows" && file.IsSymlink():
		return errSymlinksUnsupported

	case runtime.GOOS == "windows" && windowsInvalidFilename(file.FileName()):
		return fs.ErrInvalidFilename
	}

	return nil
}

var windowsDisallowedCharacters = string([]rune{
	'<', '>', ':', '"', '|', '?', '*',
	0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
	11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
	21, 22, 23, 24, 25, 26, 27, 28, 29, 30,
	31,
})

func windowsInvalidFilename(name string) bool {
	// None of the path components should end in space
	for _, part := range strings.Split(name, `\`) {
		if len(part) == 0 {
			continue
		}
		if part[len(part)-1] == ' ' {
			// Names ending in space are not valid.
			return true
		}
	}

	// The path must not contain any disallowed characters
	return strings.ContainsAny(name, windowsDisallowedCharacters)
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

// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/syncthing/syncthing/lib/changeset"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/versioner"
)

// Which filemode bits to preserve
const retainBits = os.ModeSetgid | os.ModeSetuid | os.ModeSticky

var (
	activity    = newDeviceActivity()
	errNoDevice = errors.New("peers who had this file went away, or the file has changed while syncing. will retry later")
)

const (
	defaultPullers     = 16
	defaultPullerSleep = 10 * time.Second
	defaultPullerPause = 60 * time.Second
)

type rwFolder struct {
	stateTracker

	model *Model

	folder         string
	dir            string
	scanIntv       time.Duration
	versioner      versioner.Versioner
	ignorePerms    bool
	copiers        int
	pullers        int
	shortID        protocol.ShortID
	order          config.PullOrder
	maxConflicts   int
	sleep          time.Duration
	pause          time.Duration
	checkFreeSpace bool

	stop        chan struct{}
	scanTimer   *time.Timer
	pullTimer   *time.Timer
	delayScan   chan time.Duration
	scanNow     chan rescanRequest
	remoteIndex chan struct{} // An index update was received, we should re-evaluate needs

	currentChangeSet    *changeset.ChangeSet
	currentTracker      *currentTracker
	currentChangeSetMut sync.Mutex
}

func newRWFolder(m *Model, shortID protocol.ShortID, cfg config.FolderConfiguration) *rwFolder {
	p := &rwFolder{
		stateTracker: stateTracker{
			folder: cfg.ID,
			mut:    sync.NewMutex(),
		},

		model: m,

		folder:         cfg.ID,
		dir:            cfg.Path(),
		scanIntv:       time.Duration(cfg.RescanIntervalS) * time.Second,
		ignorePerms:    cfg.IgnorePerms,
		copiers:        cfg.Copiers,
		pullers:        cfg.Pullers,
		shortID:        shortID,
		order:          cfg.Order,
		maxConflicts:   cfg.MaxConflicts,
		checkFreeSpace: cfg.MinDiskFreePct != 0,

		stop:        make(chan struct{}),
		pullTimer:   time.NewTimer(time.Second),
		scanTimer:   time.NewTimer(time.Millisecond), // The first scan should be done immediately.
		delayScan:   make(chan time.Duration),
		scanNow:     make(chan rescanRequest),
		remoteIndex: make(chan struct{}, 1), // This needs to be 1-buffered so that we queue a notification if we're busy doing a pull when it comes.

		currentChangeSetMut: sync.NewMutex(),
	}

	if p.pullers == 0 {
		p.pullers = defaultPullers
	}

	if cfg.PullerPauseS == 0 {
		p.pause = defaultPullerPause
	} else {
		p.pause = time.Duration(cfg.PullerPauseS) * time.Second
	}

	if cfg.PullerSleepS == 0 {
		p.sleep = defaultPullerSleep
	} else {
		p.sleep = time.Duration(cfg.PullerSleepS) * time.Second
	}

	return p
}

// Serve will run scans and pulls. It will return when Stop()ed or on a
// critical error.
func (p *rwFolder) Serve() {
	l.Debugln(p, "starting")
	defer l.Debugln(p, "exiting")

	defer func() {
		p.pullTimer.Stop()
		p.scanTimer.Stop()
		// TODO: Should there be an actual FolderStopped state?
		p.setState(FolderIdle)
	}()

	var prevVer int64
	var prevIgnoreHash string

	// We don't start pulling files until a scan has been completed.
	initialScanCompleted := false

	for {
		select {
		case <-p.stop:
			return

		case <-p.remoteIndex:
			prevVer = 0
			p.pullTimer.Reset(0)
			l.Debugln(p, "remote index updated, rescheduling pull")

		case <-p.pullTimer.C:
			if !initialScanCompleted {
				l.Debugln(p, "skip (initial)")
				p.pullTimer.Reset(p.sleep)
				continue
			}

			p.model.fmut.RLock()
			curIgnores := p.model.folderIgnores[p.folder]
			p.model.fmut.RUnlock()

			if newHash := curIgnores.Hash(); newHash != prevIgnoreHash {
				// The ignore patterns have changed. We need to re-evaluate if
				// there are files we need now that were ignored before.
				l.Debugln(p, "ignore patterns have changed, resetting prevVer")
				prevVer = 0
				prevIgnoreHash = newHash
			}

			// RemoteLocalVersion() is a fast call, doesn't touch the database.
			curVer, ok := p.model.RemoteLocalVersion(p.folder)
			if !ok || curVer == prevVer {
				l.Debugln(p, "skip (curVer == prevVer)", prevVer, ok)
				p.pullTimer.Reset(p.sleep)
				continue
			}

			if err := p.model.CheckFolderHealth(p.folder); err != nil {
				l.Infoln("Skipping folder", p.folder, "pull due to folder error:", err)
				p.pullTimer.Reset(p.sleep)
				continue
			}

			l.Debugln(p, "pulling", prevVer, curVer)

			p.setState(FolderSyncing)
			tries := 0

			for {
				tries++

				changed, err := p.pullerIteration(curIgnores)
				l.Debugln(p, "changed", changed)

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
					l.Debugln(p, "next pull in", p.sleep)
					p.pullTimer.Reset(p.sleep)
					break
				}

				if tries > 10 {
					// We've tried a bunch of times to get in sync, but
					// we're not making it. Probably there are write
					// errors preventing us. Flag this with a warning and
					// wait a bit longer before retrying.
					l.Infof("Folder %q isn't making progress. Pausing puller for %v.", p.folder, p.pause)
					l.Debugln(p, "next pull in", p.pause)

					if err, ok := err.(changeset.ApplyError); ok {
						for _, err := range err.Errors() {
							l.Infoln(err)
						}
						events.Default.Log(events.FolderErrors, map[string]interface{}{
							"folder": p.folder,
							"errors": err.Errors(),
						})
					} else {
						// This is weird; the error should be from
						// changeset.Apply and should be an ApplyError, but
						// lets not panic in case something else happened.
						l.Warnln(err)
					}

					p.pullTimer.Reset(p.pause)
					break
				}
			}
			p.setState(FolderIdle)

		// The reason for running the scanner from within the puller is that
		// this is the easiest way to make sure we are not doing both at the
		// same time.
		case <-p.scanTimer.C:
			err := p.scanSubsIfHealthy(nil)
			p.rescheduleScan()
			if err != nil {
				continue
			}
			if !initialScanCompleted {
				l.Infoln("Completed initial scan (rw) of folder", p.folder)
				initialScanCompleted = true
			}

		case req := <-p.scanNow:
			req.err <- p.scanSubsIfHealthy(req.subs)

		case next := <-p.delayScan:
			p.scanTimer.Reset(next)
		}
	}
}

func (p *rwFolder) rescheduleScan() {
	if p.scanIntv == 0 {
		// We should not run scans, so it should not be rescheduled.
		return
	}
	// Sleep a random time between 3/4 and 5/4 of the configured interval.
	sleepNanos := (p.scanIntv.Nanoseconds()*3 + rand.Int63n(2*p.scanIntv.Nanoseconds())) / 4
	intv := time.Duration(sleepNanos) * time.Nanosecond
	l.Debugln(p, "next rescan in", intv)
	p.scanTimer.Reset(intv)
}

func (p *rwFolder) scanSubsIfHealthy(subs []string) error {
	if err := p.model.CheckFolderHealth(p.folder); err != nil {
		l.Infoln("Skipping folder", p.folder, "scan due to folder error:", err)
		return err
	}
	l.Debugln(p, "Scanning subdirectories")
	if err := p.model.internalScanFolderSubs(p.folder, subs); err != nil {
		// Potentially sets the error twice, once in the scanner just
		// by doing a check, and once here, if the error returned is
		// the same one as returned by CheckFolderHealth, though
		// duplicate set is handled by setError.
		p.setError(err)
		return err
	}
	return nil
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
func (p *rwFolder) pullerIteration(ignores *ignore.Matcher) (int, error) {
	// Create the database updater and event emitter things, who each listen
	// to progress updates from the changeset. Also the currentTracker that we
	// use to keep track of files currently being processed.

	dbUpdater := newDatabaseUpdater(p.folder, p.model)
	defer dbUpdater.Close() // also implicitly awaits db commit of all changes
	eventEmitter := newProgressTracker(p.folder)
	currentTracker := newCurrentTracker()
	progresser := multiProgresser{
		dbUpdater,
		eventEmitter,
		currentTracker,
	}

	// Set up the local block requester.

	var folders []string
	folderRoots := make(map[string]string)

	p.model.fmut.RLock()
	for folder, cfg := range p.model.folderCfgs {
		folders = append(folders, folder)
		folderRoots[folder] = cfg.Path()
	}
	p.model.fmut.RUnlock()

	localRequester := &localBlockPuller{
		model:       p.model,
		folders:     folders,
		folderRoots: folderRoots,
	}

	// Set up the network block requester

	np := &networkBlockPuller{
		model:  p.model,
		folder: p.folder,
	}
	networkRequester := changeset.NewAsyncRequester(np, p.pullers)

	// Set up the virtual mtime store

	mtimeKVStore := db.NewNamespacedKV(p.model.db, string(db.KeyTypeVirtualMtime)+p.folder)
	filesystem := fs.NewMtimeFS(mtimeKVStore)

	// Create a new changeset to apply the changes

	cs := changeset.New(changeset.Options{
		RootPath:         p.dir,
		MaxConflicts:     p.maxConflicts,
		CurrentFiler:     cFiler{p.model, p.folder},
		TempNamer:        defTempNamer,
		Progresser:       progresser,
		Archiver:         p.versioner,
		LocalRequester:   localRequester,
		NetworkRequester: networkRequester,
		Filesystem:       filesystem,
	})

	// Grab the files we need to process.

	p.model.fmut.RLock()
	folderFiles := p.model.folderFiles[p.folder]
	p.model.fmut.RUnlock()
	folderFiles.WithNeed(protocol.LocalDeviceID, func(intf db.FileIntf) bool {
		file := intf.(protocol.FileInfo)

		if ignores.Match(file.Name) {
			// This is an ignored file. Skip it, continue iteration.
			return true
		}

		scanner.PopulateOffsets(file.Blocks)
		cs.Queue(file)

		return true
	})

	changed := cs.Size()

	// Reorder the file queue according to configuration

	switch p.order {
	case config.OrderRandom:
		cs.Shuffle()
	case config.OrderAlphabetic:
		// The queue is already in alphabetic order.
	case config.OrderSmallestFirst:
		cs.SortSmallestFirst()
	case config.OrderLargestFirst:
		cs.SortLargestFirst()
	case config.OrderOldestFirst:
		cs.SortOldestFirst()
	case config.OrderNewestFirst:
		cs.SortNewestFirst()
	}

	// Process the file queue

	// The change set is prepped. "Publish" it in currentChangeSet so that the
	// user can request order changes while Apply is running.
	p.currentChangeSetMut.Lock()
	p.currentChangeSet = cs
	p.currentTracker = currentTracker
	p.currentChangeSetMut.Unlock()

	err := cs.Apply()

	// Unpublish it once we're done.
	p.currentChangeSetMut.Lock()
	p.currentChangeSet = nil
	p.currentTracker = nil
	p.currentChangeSetMut.Unlock()

	return changed, err
}

// Moves the given filename to the front of the job queue
func (p *rwFolder) BringToFront(filename string) {
	p.currentChangeSetMut.Lock()
	cs := p.currentChangeSet
	p.currentChangeSetMut.Unlock()

	if cs != nil {
		cs.BringToFront(filename)
	}
}

func (p *rwFolder) Jobs() (inProgress []string, queued []string) {
	p.currentChangeSetMut.Lock()
	cs := p.currentChangeSet
	ct := p.currentTracker
	p.currentChangeSetMut.Unlock()

	if cs != nil {
		// p.currentChangeSet and p.currentTracker are updated together so
		// either both are nil or none are.
		return ct.Current(), cs.QueueNames()
	}
	return nil, nil
}

func (p *rwFolder) DelayScan(next time.Duration) {
	p.delayScan <- next
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

// The multiProgresser delegates each call to each of the underlying
// Progressers
type multiProgresser []changeset.Progresser

func (l multiProgresser) Started(file protocol.FileInfo) {
	for _, p := range l {
		p.Started(file)
	}
}

func (l multiProgresser) Progress(file protocol.FileInfo, copied, requested, downloaded int) {
	for _, p := range l {
		p.Progress(file, copied, requested, downloaded)
	}
}

func (l multiProgresser) Completed(file protocol.FileInfo, err error) {
	for _, p := range l {
		p.Completed(file, err)
	}
}

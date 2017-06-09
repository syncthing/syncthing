// Copyright (C) 2016 The Syncthing Authors.
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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/weakhash"
)

type rescanRequest struct {
	subdirs []string
	err     chan error
}

// A folderScanner manages scanning. It runs independently and can be
// embedded into other objects, managed by a supervisor. The embedded
// sync.Mutex will be held while a scan is in progress. Other activities
// that cannot run concurrently with a scan should acquire the same mutex
// for the duration.
type folderScanner struct {
	sync.Mutex
	cfg config.FolderConfiguration
	folderScannerConfig
	timer  *time.Timer
	now    chan rescanRequest
	delay  chan time.Duration
	ctx    context.Context
	cancel context.CancelFunc
}

type dbPrefixIterator interface {
	iterate(prefix string, iterator db.Iterator)
}

type dbUpdater interface {
	update(files []protocol.FileInfo)
}

type folderScannerConfig struct {
	// mandatory depdendencies
	shortID          protocol.ShortID
	currentFiler     scanner.CurrentFiler
	filesystem       fs.Filesystem
	ignores          *ignore.Matcher
	stateTracker     *stateTracker
	dbUpdater        dbUpdater
	dbPrefixIterator dbPrefixIterator

	// optional hooks
	ignoresChanged func()
	scanCompleted  func()
}

func newFolderScanner(ctx context.Context, cfg config.FolderConfiguration, fsCfg folderScannerConfig) *folderScanner {
	ctx, cancel := context.WithCancel(ctx)
	return &folderScanner{
		Mutex:               sync.NewMutex(),
		cfg:                 cfg,
		folderScannerConfig: fsCfg,
		ctx:                 ctx,
		cancel:              cancel,
		timer:               time.NewTimer(time.Millisecond), // The first scan should be done immediately.
		now:                 make(chan rescanRequest),
		delay:               make(chan time.Duration),
	}
}

func (f *folderScanner) Scan(subdirs []string) error {
	req := rescanRequest{
		subdirs: subdirs,
		err:     make(chan error),
	}
	f.now <- req
	return <-req.err
}

func (f *folderScanner) DelayScan(next time.Duration) {
	f.delay <- next
}

func (f *folderScanner) Serve() {
	for {
		select {
		case <-f.timer.C:
			f.scan(f.ctx, nil)
			f.reschedule()

		case req := <-f.now:
			req.err <- f.scan(f.ctx, req.subdirs)
			f.reschedule()

		case delay := <-f.delay:
			f.timer.Reset(delay)
		}
	}
}

func (f *folderScanner) Stop() {
	f.cancel()
}

func (f *folderScanner) scan(ctx context.Context, subDirs []string) (err error) {
	f.Lock()
	defer f.Unlock()

	subDirs, err = f.cleanSubDirs(subDirs)
	if err != nil {
		return err
	}

	if err := f.healthCheck(); err != nil {
		return err
	}

	oldHash := f.ignores.Hash()
	if err := f.ignores.Load(filepath.Join(f.cfg.Path(), ".stignore")); err != nil && !os.IsNotExist(err) {
		err = fmt.Errorf("loading ignores: %v", err)
		f.stateTracker.setError(err)
		l.Infof("Stopping folder %s due to error: %s", f.cfg.Description(), err)
		return err
	}

	f.stateTracker.setState(FolderScanning)

	if err := f.scanForAdditions(ctx, subDirs); err != nil {
		return err
	}
	if err := f.scanForDeletes(ctx, subDirs); err != nil {
		return err
	}

	if f.scanCompleted != nil {
		f.scanCompleted() // TODO move to the state tracker
	}
	f.stateTracker.setState(FolderIdle)

	// Check if the ignore patterns changed as part of scanning this folder.
	// If they did we should schedule a pull of the folder so that we
	// request things we might have suddenly become unignored and so on.
	if f.ignores.Hash() != oldHash && f.ignoresChanged != nil {
		f.ignoresChanged()
	}

	return nil
}

func (f *folderScanner) scanForAdditions(ctx context.Context, subDirs []string) error {
	fchan, err := scanner.Walk(ctx, scanner.Config{
		Folder:                f.cfg.ID,
		Dir:                   f.cfg.Path(),
		Subs:                  subDirs,
		Matcher:               f.ignores,
		BlockSize:             protocol.BlockSize,
		TempLifetime:          24 * time.Hour, // XXX time.Duration(m.cfg.Options().KeepTemporariesH) * time.Hour,
		CurrentFiler:          f.currentFiler,
		Filesystem:            f.filesystem,
		IgnorePerms:           f.cfg.IgnorePerms,
		AutoNormalize:         f.cfg.AutoNormalize,
		Hashers:               f.numHashers(),
		ShortID:               f.shortID,
		ProgressTickIntervalS: f.cfg.ScanProgressIntervalS,
		UseWeakHashes:         weakhash.Enabled,
	})

	if err != nil {
		// The error we get here is likely an OS level error, which might not be
		// as readable as our health check errors. Check if we can get a health
		// check error first, and use that if it's available.
		if err := f.healthCheck(); err != nil {
			return err
		}
		return err
	}

	batch := make([]protocol.FileInfo, 0, maxBatchSizeFiles)
	batchSizeBytes := 0

	for fi := range fchan {
		if len(batch) == maxBatchSizeFiles || batchSizeBytes > maxBatchSizeBytes {
			if err := f.healthCheck(); err != nil {
				return err
			}
			f.dbUpdater.update(batch)
			batch = batch[:0]
			batchSizeBytes = 0
		}
		batch = append(batch, fi)
		batchSizeBytes += fi.ProtoSize()
	}

	if len(batch) > 0 {
		if err := f.healthCheck(); err != nil {
			return err
		}
		f.dbUpdater.update(batch)
	}

	return nil
}

func (f *folderScanner) scanForDeletes(ctx context.Context, subDirs []string) error {
	if len(subDirs) == 0 {
		// If we have no specific subdirectories to traverse, set it to one
		// empty prefix so we traverse the entire folder contents once.
		subDirs = []string{""}
	}

	batch := make([]protocol.FileInfo, 0, maxBatchSizeFiles)
	batchSizeBytes := 0

	// Do a scan of the database for each prefix, to check for deleted and
	// ignored files.
	for _, sub := range subDirs {
		var innerError error
		f.dbPrefixIterator.iterate(sub, func(tfi db.FileIntf) bool {
			select {
			case <-ctx.Done():
				return false
			default:
			}

			fi := tfi.(db.FileInfoTruncated)
			if len(batch) == maxBatchSizeFiles || batchSizeBytes > maxBatchSizeBytes {
				if err := f.healthCheck(); err != nil {
					innerError = err
					return false
				}
				f.dbUpdater.update(batch)
				batch = batch[:0]
				batchSizeBytes = 0
			}

			switch {
			case !fi.IsInvalid() && f.ignores.Match(fi.Name).IsIgnored():
				// File was valid at last pass but has been ignored. Set invalid bit.
				l.Debugln("setting invalid bit on ignored", f)
				nf := protocol.FileInfo{
					Name:          fi.Name,
					Type:          fi.Type,
					Size:          fi.Size,
					ModifiedS:     fi.ModifiedS,
					ModifiedNs:    fi.ModifiedNs,
					ModifiedBy:    f.shortID,
					Permissions:   fi.Permissions,
					NoPermissions: fi.NoPermissions,
					Invalid:       true,
					Version:       fi.Version, // The file is still the same, so don't bump version
				}
				batch = append(batch, nf)
				batchSizeBytes += nf.ProtoSize()

			case !fi.IsInvalid() && !fi.IsDeleted():
				// The file is valid and not deleted. Lets check if it's
				// still here.

				if _, err := f.filesystem.Lstat(filepath.Join(f.cfg.Path(), fi.Name)); err != nil {
					// We don't specifically verify that the error is
					// os.IsNotExist because there is a corner case when a
					// directory is suddenly transformed into a file. When that
					// happens, files that were in the directory (that is now a
					// file) are deleted but will return a confusing error ("not a
					// directory") when we try to Lstat() them.

					nf := protocol.FileInfo{
						Name:       fi.Name,
						Type:       fi.Type,
						Size:       0,
						ModifiedS:  fi.ModifiedS,
						ModifiedNs: fi.ModifiedNs,
						ModifiedBy: f.shortID,
						Deleted:    true,
						Version:    fi.Version.Update(f.shortID),
					}

					batch = append(batch, nf)
					batchSizeBytes += nf.ProtoSize()
				}
			}
			return true
		})
		if innerError != nil {
			return innerError
		}
	}

	if len(batch) > 0 {
		if err := f.healthCheck(); err != nil {
			return err
		}
		f.dbUpdater.update(batch)
	}

	return nil
}

func (f *folderScanner) reschedule() {
	if f.cfg.RescanIntervalS == 0 {
		return
	}
	// Sleep a random time between 3/4 and 5/4 of the configured interval.
	interval := time.Duration(f.cfg.RescanIntervalS) * time.Second
	sleep := time.Duration(int64(interval)*3 + rand.Int63n(2*int64(interval))/4)
	l.Debugln(f, "next rescan in", sleep)
	f.timer.Reset(interval)
}

func (f *folderScanner) cleanSubDirs(subDirs []string) ([]string, error) {
	for i := 0; i < len(subDirs); i++ {
		sub := osutil.NativeFilename(subDirs[i])

		if sub == "" {
			// A blank subdirs means to scan the entire folder. We can trim
			// the subDirs list and go on our way.
			return nil, nil
		}

		// We test each path by joining with "root". What we join with is
		// not relevant, we just want the dotdot escape detection here. For
		// historical reasons we may get paths that end in a slash. We
		// remove that first to allow the rootedJoinedPath to pass.
		sub = strings.TrimRight(sub, string(os.PathSeparator))
		if _, err := rootedJoinedPath("root", sub); err != nil {
			return nil, errors.New("invalid subpath")
		}
		subDirs[i] = sub
	}

	// Clean the list of subitems to ensure that we start at a known
	// directory, and don't scan subdirectories of things we've already
	// scanned.
	subDirs = unifySubs(subDirs, func(name string) bool {
		_, ok := f.currentFiler.CurrentFile(name)
		return ok
	})

	return subDirs, nil
}

// numHashers returns the number of hasher routines to use for a given folder,
// taking into account configuration and available CPU cores.
func (f *folderScanner) numHashers() int {
	if f.cfg.Hashers > 0 {
		// Specific value set in the config, use that.
		return f.cfg.Hashers
	}

	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		// Interactive operating systems; don't load the system too heavily by
		// default.
		return 1
	}

	// For other operating systems and architectures, lets try to get some
	// work done... Use all but one CPU cores, leaving one core for
	// database, housekeeping, other tasks, ...
	if perFolder := runtime.GOMAXPROCS(-1) - 1; perFolder > 0 {
		return perFolder
	}

	return 1
}

func (f *folderScanner) healthCheck() error {
	return nil // XXX
}

func (f *folderScanner) stopWithError(err error) {
	l.Infof("Stopping folder %s due to error: %s", f.cfg.Description(), err)
	f.stateTracker.setError(err)
}

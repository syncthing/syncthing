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
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
	"github.com/syncthing/syncthing/lib/weakhash"
)

type rescanRequest struct {
	subdirs []string
	err     chan error
}

// bundle all folder scan activity
type folderScanner struct {
	cfg   config.FolderConfiguration
	timer *time.Timer
	now   chan rescanRequest
	delay chan time.Duration

	currentFiler scanner.CurrentFiler
	filesystem   fs.Filesystem
	ignores      *ignore.Matcher
	healthCheck  func() error
	updates      func([]protocol.FileInfo)
	stateTracker *stateTracker
	shortID      protocol.ShortID
}

func newFolderScanner(cfg config.FolderConfiguration) folderScanner {
	return folderScanner{
		cfg:   cfg,
		timer: time.NewTimer(time.Millisecond), // The first scan should be done immediately.
		now:   make(chan rescanRequest),
		delay: make(chan time.Duration),
	}
}

func (f *folderScanner) Reschedule() {
	if f.cfg.RescanIntervalS == 0 {
		return
	}
	// Sleep a random time between 3/4 and 5/4 of the configured interval.
	interval := time.Duration(f.cfg.RescanIntervalS) * time.Second
	sleep := time.Duration(int64(interval)*3 + rand.Int63n(2*int64(interval))/4)
	l.Debugln(f, "next rescan in", sleep)
	f.timer.Reset(interval)
}

func (f *folderScanner) Scan(subdirs []string) error {
	req := rescanRequest{
		subdirs: subdirs,
		err:     make(chan error),
	}
	f.now <- req
	return <-req.err
}

func (f *folderScanner) Delay(next time.Duration) {
	f.delay <- next
}

func (f *folderScanner) HasNoInterval() bool {
	return f.cfg.RescanIntervalS == 0
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
}

func (f *folderScanner) internalScanFolderSubdirs(ctx context.Context, folder string, subDirs []string) error {
	subDirs, err := f.cleanSubDirs(subDirs)
	if err != nil {
		return err
	}

	// Check if the ignore patterns changed as part of scanning this folder.
	// If they did we should schedule a pull of the folder so that we
	// request things we might have suddenly become unignored and so on.

	oldHash := f.ignores.Hash()
	defer func() {
		if f.ignores.Hash() != oldHash {
			l.Debugln("Folder", folder, "ignore patterns changed; triggering puller")
			f.stateTracker.IndexUpdated()
		}
	}()

	if err := f.healthCheck(); err != nil {
		f.stateTracker.setError(err)
		l.Infof("Stopping folder %s due to error: %s", f.cfg.Description(), err)
		return err
	}

	if err := f.ignores.Load(filepath.Join(f.cfg.Path(), ".stignore")); err != nil && !os.IsNotExist(err) {
		err = fmt.Errorf("loading ignores: %v", err)
		f.stateTracker.setError(err)
		l.Infof("Stopping folder %s due to error: %s", f.cfg.Description(), err)
		return err
	}

	f.stateTracker.setState(FolderScanning)

	fchan, err := scanner.Walk(ctx, scanner.Config{
		Folder:                f.cfg.ID,
		Dir:                   f.cfg.Path(),
		Subs:                  subDirs,
		Matcher:               f.ignores,
		BlockSize:             protocol.BlockSize,
		TempLifetime:          time.Duration(m.cfg.Options().KeepTemporariesH) * time.Hour,
		CurrentFiler:          cFiler{m, folder},
		Filesystem:            f.filesystem,
		IgnorePerms:           f.cfg.IgnorePerms,
		AutoNormalize:         f.cfg.AutoNormalize,
		Hashers:               m.numHashers(folder),
		ShortID:               f.shortID,
		ProgressTickIntervalS: f.cfg.ScanProgressIntervalS,
		UseWeakHashes:         weakhash.Enabled,
	})

	if err != nil {
		// The error we get here is likely an OS level error, which might not be
		// as readable as our health check errors. Check if we can get a health
		// check error first, and use that if it's available.
		if ferr := f.healthCheck(); ferr != nil {
			err = ferr
		}
		f.stateTracker.setError(err)
		return err
	}

	batch := make([]protocol.FileInfo, 0, maxBatchSizeFiles)
	batchSizeBytes := 0

	for fi := range fchan {
		if len(batch) == maxBatchSizeFiles || batchSizeBytes > maxBatchSizeBytes {
			if err := f.healthCheck(); err != nil {
				l.Infof("Stopping folder %s mid-scan due to folder error: %s", f.cfg.Description(), err)
				return err
			}
			f.updates(batch)
			batch = batch[:0]
			batchSizeBytes = 0
		}
		batch = append(batch, fi)
		batchSizeBytes += fi.ProtoSize()
	}

	if err := f.healthCheck(); err != nil {
		l.Infof("Stopping folder %s mid-scan due to folder error: %s", f.cfg.Description(), err)
		return err
	} else if len(batch) > 0 {
		f.updates(batch)
	}

	if len(subDirs) == 0 {
		// If we have no specific subdirectories to traverse, set it to one
		// empty prefix so we traverse the entire folder contents once.
		subDirs = []string{""}
	}

	// Do a scan of the database for each prefix, to check for deleted and
	// ignored files.
	batch = batch[:0]
	batchSizeBytes = 0
	for _, sub := range subDirs {
		var iterError error

		fs.WithPrefixedHaveTruncated(protocol.LocalDeviceID, sub, func(tfi db.FileIntf) bool {
			fi := tfi.(db.FileInfoTruncated)
			if len(batch) == maxBatchSizeFiles || batchSizeBytes > maxBatchSizeBytes {
				if err := f.healthCheck(); err != nil {
					iterError = err
					return false
				}
				f.updates(batch)
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

		if iterError != nil {
			l.Infof("Stopping folder %s mid-scan due to folder error: %s", f.cfg.Description(), iterError)
			return iterError
		}
	}

	if err := f.healthCheck(); err != nil {
		l.Infof("Stopping folder %s mid-scan due to folder error: %s", f.cfg.Description(), err)
		return err
	} else if len(batch) > 0 {
		f.updates(batch)
	}

	// XXX m.folderStatRef(folder).ScanCompleted()
	f.stateTracker.setState(FolderIdle)
	return nil
}

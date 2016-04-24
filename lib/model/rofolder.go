// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"
	"time"

	"github.com/syncthing/syncthing/lib/sync"
)

type roFolder struct {
	stateTracker
	scan

	folderId string
	model    *Model
	stop     chan struct{}
}

func newROFolder(model *Model, folderId string, scanInterval time.Duration) *roFolder {
	return &roFolder{
		stateTracker: stateTracker{
			folderID: folderId,
			mut:      sync.NewMutex(),
		},
		scan: scan{
			scanInterval: scanInterval,
			scanTimer:    time.NewTimer(time.Millisecond),
			scanNow:      make(chan rescanRequest),
			scanDelay:    make(chan time.Duration),
		},
		folderId: folderId,
		model:    model,
		stop:     make(chan struct{}),
	}
}

func (f *roFolder) Serve() {
	l.Debugln(f, "starting")
	defer l.Debugln(f, "exiting")

	defer func() {
		f.scanTimer.Stop()
	}()

	initialScanCompleted := false
	for {
		select {
		case <-f.stop:
			return

		case <-f.scanTimer.C:
			if err := f.model.CheckFolderHealth(f.folderId); err != nil {
				l.Infoln("Skipping folder", f.folderId, "scan due to folder error:", err)
				f.rescheduleScan()
				continue
			}

			l.Debugln(f, "rescan")

			if err := f.model.internalScanFolderSubdirs(f.folderId, nil); err != nil {
				// Potentially sets the error twice, once in the scanner just
				// by doing a check, and once here, if the error returned is
				// the same one as returned by CheckFolderHealth, though
				// duplicate set is handled by setError.
				f.setError(err)
				f.rescheduleScan()
				continue
			}

			if !initialScanCompleted {
				l.Infoln("Completed initial scan (ro) of folder", f.folderId)
				initialScanCompleted = true
			}

			if f.scanInterval == 0 {
				continue
			}

			f.rescheduleScan()

		case req := <-f.scanNow:
			if err := f.model.CheckFolderHealth(f.folderId); err != nil {
				l.Infoln("Skipping folder", f.folderId, "scan due to folder error:", err)
				req.err <- err
				continue
			}

			l.Debugln(f, "forced rescan")

			if err := f.model.internalScanFolderSubdirs(f.folderId, req.subdirs); err != nil {
				// Potentially sets the error twice, once in the scanner just
				// by doing a check, and once here, if the error returned is
				// the same one as returned by CheckFolderHealth, though
				// duplicate set is handled by setError.
				f.setError(err)
				req.err <- err
				continue
			}

			req.err <- nil

		case next := <-f.scanDelay:
			f.scanTimer.Reset(next)
		}
	}
}

func (f *roFolder) Stop() {
	close(f.stop)
}

func (f *roFolder) IndexUpdated() {
}

func (f *roFolder) String() string {
	return fmt.Sprintf("roFolder/%s@%p", f.folderId, f)
}

func (f *roFolder) BringToFront(string) {}

func (f *roFolder) Jobs() ([]string, []string) {
	return nil, nil
}

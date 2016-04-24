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
	scan folderscan

	folderID string
	model    *Model
	stop     chan struct{}
}

func newROFolder(model *Model, folderID string, scanInterval time.Duration) *roFolder {
	return &roFolder{
		stateTracker: stateTracker{
			folderID: folderID,
			mut:      sync.NewMutex(),
		},
		scan: folderscan{
			interval: scanInterval,
			timer:    time.NewTimer(time.Millisecond),
			now:      make(chan rescanRequest),
			delay:    make(chan time.Duration),
		},
		folderID: folderID,
		model:    model,
		stop:     make(chan struct{}),
	}
}

func (f *roFolder) Serve() {
	l.Debugln(f, "starting")
	defer l.Debugln(f, "exiting")

	defer func() {
		f.scan.timer.Stop()
	}()

	initialScanCompleted := false
	for {
		select {
		case <-f.stop:
			return

		case <-f.scan.timer.C:
			if err := f.model.CheckFolderHealth(f.folderID); err != nil {
				l.Infoln("Skipping folder", f.folderID, "scan due to folder error:", err)
				f.scan.reschedule()
				continue
			}

			l.Debugln(f, "rescan")

			if err := f.model.internalScanFolderSubdirs(f.folderID, nil); err != nil {
				// Potentially sets the error twice, once in the scanner just
				// by doing a check, and once here, if the error returned is
				// the same one as returned by CheckFolderHealth, though
				// duplicate set is handled by setError.
				f.setError(err)
				f.scan.reschedule()
				continue
			}

			if !initialScanCompleted {
				l.Infoln("Completed initial scan (ro) of folder", f.folderID)
				initialScanCompleted = true
			}

			if f.scan.interval == 0 {
				continue
			}

			f.scan.reschedule()

		case req := <-f.scan.now:
			if err := f.model.CheckFolderHealth(f.folderID); err != nil {
				l.Infoln("Skipping folder", f.folderID, "scan due to folder error:", err)
				req.err <- err
				continue
			}

			l.Debugln(f, "forced rescan")

			if err := f.model.internalScanFolderSubdirs(f.folderID, req.subdirs); err != nil {
				// Potentially sets the error twice, once in the scanner just
				// by doing a check, and once here, if the error returned is
				// the same one as returned by CheckFolderHealth, though
				// duplicate set is handled by setError.
				f.setError(err)
				req.err <- err
				continue
			}

			req.err <- nil

		case next := <-f.scan.delay:
			f.scan.timer.Reset(next)
		}
	}
}

func (f *roFolder) Stop() {
	close(f.stop)
}

func (f *roFolder) IndexUpdated() {
}

func (f *roFolder) String() string {
	return fmt.Sprintf("roFolder/%s@%p", f.folderID, f)
}

func (f *roFolder) BringToFront(string) {}

func (f *roFolder) Jobs() ([]string, []string) {
	return nil, nil
}

func (f *roFolder) DelayScan(next time.Duration) {
	f.scan.Delay(next)
}

func (f *roFolder) Scan(subdirs []string) error {
	return f.scan.Scan(subdirs)
}

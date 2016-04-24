// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"
	"math/rand"
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

type rescanRequest struct {
	subdirs []string
	err     chan error
}

// bundle all folder scan activity
type scan struct {
	scanInterval time.Duration
	scanTimer    *time.Timer
	scanNow      chan rescanRequest
	scanDelay    chan time.Duration
}

func (scan *scan) rescheduleScan() {
	if scan.scanInterval == 0 {
		return
	}
	// Sleep a random time between 3/4 and 5/4 of the configured interval.
	sleepNanos := (scan.scanInterval.Nanoseconds()*3 + rand.Int63n(2*scan.scanInterval.Nanoseconds())) / 4
	interval := time.Duration(sleepNanos) * time.Nanosecond
	l.Debugln(scan, "next rescan in", interval)
	scan.scanTimer.Reset(interval)
}

func (s *scan) Scan(subdirs []string) error {
	req := rescanRequest{
		subdirs: subdirs,
		err:     make(chan error),
	}
	s.scanNow <- req
	return <-req.err
}

func (s *scan) DelayScan(next time.Duration) {
	s.scanDelay <- next
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

func (folder *roFolder) Serve() {
	l.Debugln(folder, "starting")
	defer l.Debugln(folder, "exiting")

	defer func() {
		folder.scanTimer.Stop()
	}()

	initialScanCompleted := false
	for {
		select {
		case <-folder.stop:
			return

		case <-folder.scanTimer.C:
			if err := folder.model.CheckFolderHealth(folder.folderId); err != nil {
				l.Infoln("Skipping folder", folder.folderId, "scan due to folder error:", err)
				folder.rescheduleScan()
				continue
			}

			l.Debugln(folder, "rescan")

			if err := folder.model.internalScanFolderSubdirs(folder.folderId, nil); err != nil {
				// Potentially sets the error twice, once in the scanner just
				// by doing a check, and once here, if the error returned is
				// the same one as returned by CheckFolderHealth, though
				// duplicate set is handled by setError.
				folder.setError(err)
				folder.rescheduleScan()
				continue
			}

			if !initialScanCompleted {
				l.Infoln("Completed initial scan (ro) of folder", folder.folderId)
				initialScanCompleted = true
			}

			if folder.scanInterval == 0 {
				continue
			}

			folder.rescheduleScan()

		case req := <-folder.scanNow:
			if err := folder.model.CheckFolderHealth(folder.folderId); err != nil {
				l.Infoln("Skipping folder", folder.folderId, "scan due to folder error:", err)
				req.err <- err
				continue
			}

			l.Debugln(folder, "forced rescan")

			if err := folder.model.internalScanFolderSubdirs(folder.folderId, req.subdirs); err != nil {
				// Potentially sets the error twice, once in the scanner just
				// by doing a check, and once here, if the error returned is
				// the same one as returned by CheckFolderHealth, though
				// duplicate set is handled by setError.
				folder.setError(err)
				req.err <- err
				continue
			}

			req.err <- nil

		case next := <-folder.scanDelay:
			folder.scanTimer.Reset(next)
		}
	}
}

func (s *roFolder) Stop() {
	close(s.stop)
}

func (s *roFolder) IndexUpdated() {
}

func (s *roFolder) String() string {
	return fmt.Sprintf("roFolder/%s@%p", s.folderId, s)
}

func (s *roFolder) BringToFront(string) {}

func (s *roFolder) Jobs() ([]string, []string) {
	return nil, nil
}

func (s *roFolder) DelayScan(next time.Duration) {
	s.scanDelay <- next
}

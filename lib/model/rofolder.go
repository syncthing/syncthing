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

	folderId     string
	scanInterval time.Duration
	scanTimer    *time.Timer
	model        *Model
	stop         chan struct{}
	scanNow      chan rescanRequest
	delayScan    chan time.Duration
}

type rescanRequest struct {
	subdirs []string
	err     chan error
}

func newROFolder(model *Model, folderId string, scanInterval time.Duration) *roFolder {
	return &roFolder{
		stateTracker: stateTracker{
			folderId: folderId,
			mut:      sync.NewMutex(),
		},
		model:        model,
		folderId:     folderId,
		scanInterval: scanInterval,
		scanTimer:    time.NewTimer(time.Millisecond),
		stop:         make(chan struct{}),
		scanNow:      make(chan rescanRequest),
		delayScan:    make(chan time.Duration),
	}
}

func (folder *roFolder) Serve() {
	l.Debugln(folder, "starting")
	defer l.Debugln(folder, "exiting")

	defer func() {
		folder.scanTimer.Stop()
	}()

	reschedule := func() {
		if folder.scanInterval == 0 {
			return
		}
		// Sleep a random time between 3/4 and 5/4 of the configured interval.
		sleepNanos := (folder.scanInterval.Nanoseconds()*3 + rand.Int63n(2*folder.scanInterval.Nanoseconds())) / 4
		folder.scanTimer.Reset(time.Duration(sleepNanos) * time.Nanosecond)
	}

	initialScanCompleted := false
	for {
		select {
		case <-folder.stop:
			return

		case <-folder.scanTimer.C:
			if err := folder.model.CheckFolderHealth(folder.folderId); err != nil {
				l.Infoln("Skipping folder", folder.folderId, "scan due to folder error:", err)
				reschedule()
				continue
			}

			l.Debugln(folder, "rescan")

			if err := folder.model.internalScanFolderSubdirs(folder.folderId, nil); err != nil {
				// Potentially sets the error twice, once in the scanner just
				// by doing a check, and once here, if the error returned is
				// the same one as returned by CheckFolderHealth, though
				// duplicate set is handled by setError.
				folder.setError(err)
				reschedule()
				continue
			}

			if !initialScanCompleted {
				l.Infoln("Completed initial scan (ro) of folder", folder.folderId)
				initialScanCompleted = true
			}

			if folder.scanInterval == 0 {
				continue
			}

			reschedule()

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

		case next := <-folder.delayScan:
			folder.scanTimer.Reset(next)
		}
	}
}

func (s *roFolder) Stop() {
	close(s.stop)
}

func (s *roFolder) IndexUpdated() {
}

func (s *roFolder) Scan(subdirs []string) error {
	req := rescanRequest{
		subdirs: subdirs,
		err:     make(chan error),
	}
	s.scanNow <- req
	return <-req.err
}

func (s *roFolder) String() string {
	return fmt.Sprintf("roFolder/%s@%p", s.folderId, s)
}

func (s *roFolder) BringToFront(string) {}

func (s *roFolder) Jobs() ([]string, []string) {
	return nil, nil
}

func (s *roFolder) DelayScan(next time.Duration) {
	s.delayScan <- next
}

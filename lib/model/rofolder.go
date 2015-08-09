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

	folder    string
	intv      time.Duration
	timer     *time.Timer
	model     *Model
	stop      chan struct{}
	scanNow   chan rescanRequest
	delayScan chan time.Duration
}

type rescanRequest struct {
	subs []string
	err  chan error
}

func newROFolder(model *Model, folder string, interval time.Duration) *roFolder {
	return &roFolder{
		stateTracker: stateTracker{
			folder: folder,
			mut:    sync.NewMutex(),
		},
		folder:    folder,
		intv:      interval,
		timer:     time.NewTimer(time.Millisecond),
		model:     model,
		stop:      make(chan struct{}),
		scanNow:   make(chan rescanRequest),
		delayScan: make(chan time.Duration),
	}
}

func (s *roFolder) Serve() {
	if debug {
		l.Debugln(s, "starting")
		defer l.Debugln(s, "exiting")
	}

	defer func() {
		s.timer.Stop()
	}()

	reschedule := func() {
		if s.intv == 0 {
			return
		}
		// Sleep a random time between 3/4 and 5/4 of the configured interval.
		sleepNanos := (s.intv.Nanoseconds()*3 + rand.Int63n(2*s.intv.Nanoseconds())) / 4
		s.timer.Reset(time.Duration(sleepNanos) * time.Nanosecond)
	}

	initialScanCompleted := false
	for {
		select {
		case <-s.stop:
			return

		case <-s.timer.C:
			if err := s.model.CheckFolderHealth(s.folder); err != nil {
				l.Infoln("Skipping folder", s.folder, "scan due to folder error:", err)
				reschedule()
				continue
			}

			if debug {
				l.Debugln(s, "rescan")
			}

			if err := s.model.internalScanFolderSubs(s.folder, nil); err != nil {
				// Potentially sets the error twice, once in the scanner just
				// by doing a check, and once here, if the error returned is
				// the same one as returned by CheckFolderHealth, though
				// duplicate set is handled by setError.
				s.setError(err)
				reschedule()
				continue
			}

			if !initialScanCompleted {
				l.Infoln("Completed initial scan (ro) of folder", s.folder)
				initialScanCompleted = true
			}

			if s.intv == 0 {
				continue
			}

			reschedule()

		case req := <-s.scanNow:
			if err := s.model.CheckFolderHealth(s.folder); err != nil {
				l.Infoln("Skipping folder", s.folder, "scan due to folder error:", err)
				req.err <- err
				continue
			}

			if debug {
				l.Debugln(s, "forced rescan")
			}

			if err := s.model.internalScanFolderSubs(s.folder, req.subs); err != nil {
				// Potentially sets the error twice, once in the scanner just
				// by doing a check, and once here, if the error returned is
				// the same one as returned by CheckFolderHealth, though
				// duplicate set is handled by setError.
				s.setError(err)
				req.err <- err
				continue
			}

			req.err <- nil

		case next := <-s.delayScan:
			s.timer.Reset(next)
		}
	}
}

func (s *roFolder) Stop() {
	close(s.stop)
}

func (s *roFolder) IndexUpdated() {
}

func (s *roFolder) Scan(subs []string) error {
	req := rescanRequest{
		subs: subs,
		err:  make(chan error),
	}
	s.scanNow <- req
	return <-req.err
}

func (s *roFolder) String() string {
	return fmt.Sprintf("roFolder/%s@%p", s.folder, s)
}

func (s *roFolder) BringToFront(string) {}

func (s *roFolder) Jobs() ([]string, []string) {
	return nil, nil
}

func (s *roFolder) DelayScan(next time.Duration) {
	s.delayScan <- next
}

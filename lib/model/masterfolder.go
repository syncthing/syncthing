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

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/sync"
)

type masterFolder struct {
	stateTracker

	model *Model

	folder   string
	scanIntv time.Duration
	shortID  uint64

	stop      chan struct{}
	scanTimer *time.Timer
	delayScan chan time.Duration
	scanNow   chan rescanRequest
}

func newMasterFolder(m *Model, shortID uint64, cfg config.FolderConfiguration) *masterFolder {
	return &masterFolder{
		stateTracker: stateTracker{
			folder: cfg.ID,
			mut:    sync.NewMutex(),
		},

		model: m,

		folder:   cfg.ID,
		scanIntv: time.Duration(cfg.RescanIntervalS) * time.Second,
		shortID:  shortID,

		stop:      make(chan struct{}),
		scanTimer: time.NewTimer(time.Millisecond),
		delayScan: make(chan time.Duration),
		scanNow:   make(chan rescanRequest),
	}
}

func (s *masterFolder) Serve() {
	l.Debugln(s, "starting")
	defer l.Debugln(s, "exiting")

	defer func() {
		s.scanTimer.Stop()
	}()

	reschedule := func() {
		if s.scanIntv == 0 {
			return
		}
		// Sleep a random time between 3/4 and 5/4 of the configured interval.
		sleepNanos := (s.scanIntv.Nanoseconds()*3 + rand.Int63n(2*s.scanIntv.Nanoseconds())) / 4
		s.scanTimer.Reset(time.Duration(sleepNanos) * time.Nanosecond)
	}

	initialScanCompleted := false
	for {
		select {
		case <-s.stop:
			return

		case <-s.scanTimer.C:
			if err := s.model.CheckFolderHealth(s.folder); err != nil {
				l.Infoln("Skipping folder", s.folder, "scan due to folder error:", err)
				reschedule()
				continue
			}

			l.Debugln(s, "rescan")

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

			if s.scanIntv == 0 {
				continue
			}

			reschedule()

		case req := <-s.scanNow:
			if err := s.model.CheckFolderHealth(s.folder); err != nil {
				l.Infoln("Skipping folder", s.folder, "scan due to folder error:", err)
				req.err <- err
				continue
			}

			l.Debugln(s, "forced rescan")

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
			s.scanTimer.Reset(next)
		}
	}
}

func (s *masterFolder) Stop() {
	close(s.stop)
}

func (s *masterFolder) IndexUpdated() {
}

func (s *masterFolder) Scan(subs []string) error {
	req := rescanRequest{
		subs: subs,
		err:  make(chan error),
	}
	s.scanNow <- req
	return <-req.err
}

func (s *masterFolder) String() string {
	return fmt.Sprintf("roFolder/%s@%p", s.folder, s)
}

func (s *masterFolder) BringToFront(string) {}

func (s *masterFolder) Jobs() ([]string, []string) {
	return nil, nil
}

func (s *masterFolder) DelayScan(next time.Duration) {
	s.delayScan <- next
}

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
)

type roFolder struct {
	stateTracker

	folder string
	intv   time.Duration
	model  *Model
	stop   chan struct{}
}

func newROFolder(model *Model, folder string, interval time.Duration) *roFolder {
	return &roFolder{
		stateTracker: stateTracker{folder: folder},
		folder:       folder,
		intv:         interval,
		model:        model,
		stop:         make(chan struct{}),
	}
}

func (s *roFolder) Serve() {
	if debug {
		l.Debugln(s, "starting")
		defer l.Debugln(s, "exiting")
	}

	timer := time.NewTimer(time.Millisecond)
	defer timer.Stop()

	reschedule := func() {
		// Sleep a random time between 3/4 and 5/4 of the configured interval.
		sleepNanos := (s.intv.Nanoseconds()*3 + rand.Int63n(2*s.intv.Nanoseconds())) / 4
		timer.Reset(time.Duration(sleepNanos) * time.Nanosecond)
	}

	initialScanCompleted := false
	for {
		select {
		case <-s.stop:
			return

		case <-timer.C:
			if err := s.model.CheckFolderHealth(s.folder); err != nil {
				l.Infoln("Skipping folder", s.folder, "scan due to folder error:", err)
				reschedule()
				continue
			}

			if debug {
				l.Debugln(s, "rescan")
			}

			if err := s.model.ScanFolder(s.folder); err != nil {
				// Potentially sets the error twice, once in the scanner just
				// by doing a check, and once here, if the error returned is
				// the same one as returned by CheckFolderHealth, though
				// duplicate set is handled by SetFolderError
				s.model.cfg.SetFolderError(s.folder, err)
				reschedule()
				continue
			}

			if !initialScanCompleted {
				l.Infoln("Completed initial scan (ro) of folder", s.folder)
				initialScanCompleted = true
			}

			if s.intv == 0 {
				return
			}

			reschedule()
		}
	}
}

func (s *roFolder) Stop() {
	close(s.stop)
}

func (s *roFolder) String() string {
	return fmt.Sprintf("roFolder/%s@%p", s.folder, s)
}

func (s *roFolder) BringToFront(string) {}

func (s *roFolder) Jobs() ([]string, []string) {
	return nil, nil
}

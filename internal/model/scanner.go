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

type Scanner struct {
	folder string
	intv   time.Duration
	model  *Model
	stop   chan struct{}
}

func (s *Scanner) Serve() {
	if debug {
		l.Debugln(s, "starting")
		defer l.Debugln(s, "exiting")
	}

	timer := time.NewTimer(time.Millisecond)
	defer timer.Stop()

	initialScanCompleted := false
	for {
		select {
		case <-s.stop:
			return

		case <-timer.C:
			if debug {
				l.Debugln(s, "rescan")
			}

			s.model.setState(s.folder, FolderScanning)
			if err := s.model.ScanFolder(s.folder); err != nil {
				s.model.cfg.InvalidateFolder(s.folder, err.Error())
				return
			}
			s.model.setState(s.folder, FolderIdle)

			if !initialScanCompleted {
				l.Infoln("Completed initial scan (ro) of folder", s.folder)
				initialScanCompleted = true
			}

			if s.intv == 0 {
				return
			}

			// Sleep a random time between 3/4 and 5/4 of the configured interval.
			sleepNanos := (s.intv.Nanoseconds()*3 + rand.Int63n(2*s.intv.Nanoseconds())) / 4
			timer.Reset(time.Duration(sleepNanos) * time.Nanosecond)
		}
	}
}

func (s *Scanner) Stop() {
	close(s.stop)
}

func (s *Scanner) String() string {
	return fmt.Sprintf("scanner/%s@%p", s.folder, s)
}

func (s *Scanner) BringToFront(string) {}

func (s *Scanner) Jobs() ([]string, []string) {
	return nil, nil
}

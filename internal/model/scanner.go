// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package model

import (
	"fmt"
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
				invalidateFolder(s.model.cfg, s.folder, err)
				return
			}
			s.model.setState(s.folder, FolderIdle)

			if !initialScanCompleted {
				l.Infoln("Completed initial scan (ro) of folder", s.folder)
				initialScanCompleted = true
			}

			timer.Reset(s.intv)
		}
	}
}

func (s *Scanner) Stop() {
	close(s.stop)
}

func (s *Scanner) String() string {
	return fmt.Sprintf("scanner/%s@%p", s.folder, s)
}

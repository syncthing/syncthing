// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

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
				s.model.cfg.InvalidateFolder(s.folder, err.Error())
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

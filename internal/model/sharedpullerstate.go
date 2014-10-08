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
	"os"
	"path/filepath"
	"sync"

	"github.com/syncthing/syncthing/internal/protocol"
)

// A sharedPullerState is kept for each file that is being synced and is kept
// updated along the way.
type sharedPullerState struct {
	// Immutable, does not require locking
	file     protocol.FileInfo
	folder   string
	tempName string
	realName string
	reuse    bool

	// Mutable, must be locked for access
	err        error      // The first error we hit
	fd         *os.File   // The fd of the temp file
	copyNeeded int        // Number of copy actions we expect to happen
	pullNeeded int        // Number of block pulls we expect to happen
	closed     bool       // Set when the file has been closed
	mut        sync.Mutex // Protects the above
}

// tempFile returns the fd for the temporary file, reusing an open fd
// or creating the file as necessary.
func (s *sharedPullerState) tempFile() (*os.File, error) {
	s.mut.Lock()
	defer s.mut.Unlock()

	// If we've already hit an error, return early
	if s.err != nil {
		return nil, s.err
	}

	// If the temp file is already open, return the file descriptor
	if s.fd != nil {
		return s.fd, nil
	}

	// Ensure that the parent directory is writable. This is
	// osutil.InWritableDir except we need to do more stuff so we duplicate it
	// here.
	dir := filepath.Dir(s.tempName)
	if info, err := os.Stat(dir); err != nil {
		s.earlyCloseLocked("dst stat dir", err)
		return nil, err
	} else if info.Mode()&04 == 0 {
		err := os.Chmod(dir, 0755)
		if err == nil {
			defer func() {
				err := os.Chmod(dir, info.Mode().Perm())
				if err != nil {
					panic(err)
				}
			}()
		}
	}

	// Attempt to create the temp file
	flags := os.O_WRONLY
	if !s.reuse {
		flags |= os.O_CREATE | os.O_EXCL
	}
	fd, err := os.OpenFile(s.tempName, flags, 0644)
	if err != nil {
		s.earlyCloseLocked("dst create", err)
		return nil, err
	}

	// Same fd will be used by all writers
	s.fd = fd

	return fd, nil
}

// sourceFile opens the existing source file for reading
func (s *sharedPullerState) sourceFile() (*os.File, error) {
	s.mut.Lock()
	defer s.mut.Unlock()

	// If we've already hit an error, return early
	if s.err != nil {
		return nil, s.err
	}

	// Attempt to open the existing file
	fd, err := os.Open(s.realName)
	if err != nil {
		s.earlyCloseLocked("src open", err)
		return nil, err
	}

	return fd, nil
}

// earlyClose prints a warning message composed of the context and
// error, and marks the sharedPullerState as failed. Is a no-op when called on
// an already failed state.
func (s *sharedPullerState) earlyClose(context string, err error) {
	s.mut.Lock()
	defer s.mut.Unlock()

	s.earlyCloseLocked(context, err)
}

func (s *sharedPullerState) earlyCloseLocked(context string, err error) {
	if s.err != nil {
		return
	}

	l.Infof("Puller (folder %q, file %q): %s: %v", s.folder, s.file.Name, context, err)
	s.err = err
	if s.fd != nil {
		s.fd.Close()
		os.Remove(s.tempName)
	}
	s.closed = true
}

func (s *sharedPullerState) failed() error {
	s.mut.Lock()
	defer s.mut.Unlock()

	return s.err
}

func (s *sharedPullerState) copyDone() {
	s.mut.Lock()
	s.copyNeeded--
	if debug {
		l.Debugln("sharedPullerState", s.folder, s.file.Name, "copyNeeded ->", s.copyNeeded)
	}
	s.mut.Unlock()
}

func (s *sharedPullerState) pullStarted() {
	s.mut.Lock()
	s.pullNeeded++
	if debug {
		l.Debugln("sharedPullerState", s.folder, s.file.Name, "pullNeeded start ->", s.pullNeeded)
	}
	s.mut.Unlock()
}

func (s *sharedPullerState) pullDone() {
	s.mut.Lock()
	s.pullNeeded--
	if debug {
		l.Debugln("sharedPullerState", s.folder, s.file.Name, "pullNeeded done ->", s.pullNeeded)
	}
	s.mut.Unlock()
}

// finalClose atomically closes and returns closed status of a file. A true
// first return value means the file was closed and should be finished, with
// the error indicating the success or failure of the close. A false first
// return value indicates the file is not ready to be closed, or is already
// closed and should in either case not be finished off now.
func (s *sharedPullerState) finalClose() (bool, error) {
	s.mut.Lock()
	defer s.mut.Unlock()

	if s.pullNeeded+s.copyNeeded != 0 {
		// Not done yet.
		return false, nil
	}
	if s.closed {
		// Already handled.
		return false, nil
	}

	s.closed = true
	if fd := s.fd; fd != nil {
		s.fd = nil
		return true, fd.Close()
	}
	return true, nil
}

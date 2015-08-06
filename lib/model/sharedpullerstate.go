// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"io"
	"os"
	"path/filepath"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/sync"
)

// A sharedPullerState is kept for each file that is being synced and is kept
// updated along the way.
type sharedPullerState struct {
	// Immutable, does not require locking
	file        protocol.FileInfo // The new file (desired end state)
	folder      string
	tempName    string
	realName    string
	reused      int // Number of blocks reused from temporary file
	ignorePerms bool
	version     protocol.Vector // The current (old) version

	// Mutable, must be locked for access
	err        error      // The first error we hit
	fd         *os.File   // The fd of the temp file
	copyTotal  int        // Total number of copy actions for the whole job
	pullTotal  int        // Total number of pull actions for the whole job
	copyOrigin int        // Number of blocks copied from the original file
	copyNeeded int        // Number of copy actions still pending
	pullNeeded int        // Number of block pulls still pending
	closed     bool       // True if the file has been finalClosed.
	mut        sync.Mutex // Protects the above
}

// A momentary state representing the progress of the puller
type pullerProgress struct {
	Total               int   `json:"total"`
	Reused              int   `json:"reused"`
	CopiedFromOrigin    int   `json:"copiedFromOrigin"`
	CopiedFromElsewhere int   `json:"copiedFromElsewhere"`
	Pulled              int   `json:"pulled"`
	Pulling             int   `json:"pulling"`
	BytesDone           int64 `json:"bytesDone"`
	BytesTotal          int64 `json:"bytesTotal"`
}

// A lockedWriterAt synchronizes WriteAt calls with an external mutex.
// WriteAt() is goroutine safe by itself, but not against for example Close().
type lockedWriterAt struct {
	mut *sync.Mutex
	wr  io.WriterAt
}

func (w lockedWriterAt) WriteAt(p []byte, off int64) (n int, err error) {
	(*w.mut).Lock()
	defer (*w.mut).Unlock()
	return w.wr.WriteAt(p, off)
}

// tempFile returns the fd for the temporary file, reusing an open fd
// or creating the file as necessary.
func (s *sharedPullerState) tempFile() (io.WriterAt, error) {
	s.mut.Lock()
	defer s.mut.Unlock()

	// If we've already hit an error, return early
	if s.err != nil {
		return nil, s.err
	}

	// If the temp file is already open, return the file descriptor
	if s.fd != nil {
		return lockedWriterAt{&s.mut, s.fd}, nil
	}

	// Ensure that the parent directory is writable. This is
	// osutil.InWritableDir except we need to do more stuff so we duplicate it
	// here.
	dir := filepath.Dir(s.tempName)
	if info, err := os.Stat(dir); err != nil {
		s.failLocked("dst stat dir", err)
		return nil, err
	} else if info.Mode()&0200 == 0 {
		err := os.Chmod(dir, 0755)
		if !s.ignorePerms && err == nil {
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
	if s.reused == 0 {
		flags |= os.O_CREATE | os.O_EXCL
	} else {
		// With sufficiently bad luck when exiting or crashing, we may have
		// had time to chmod the temp file to read only state but not yet
		// moved it to it's final name. This leaves us with a read only temp
		// file that we're going to try to reuse. To handle that, we need to
		// make sure we have write permissions on the file before opening it.
		err := os.Chmod(s.tempName, 0644)
		if !s.ignorePerms && err != nil {
			s.failLocked("dst create chmod", err)
			return nil, err
		}
	}
	fd, err := os.OpenFile(s.tempName, flags, 0666)
	if err != nil {
		s.failLocked("dst create", err)
		return nil, err
	}

	// Same fd will be used by all writers
	s.fd = fd

	return lockedWriterAt{&s.mut, s.fd}, nil
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
		s.failLocked("src open", err)
		return nil, err
	}

	return fd, nil
}

// earlyClose prints a warning message composed of the context and
// error, and marks the sharedPullerState as failed. Is a no-op when called on
// an already failed state.
func (s *sharedPullerState) fail(context string, err error) {
	s.mut.Lock()
	defer s.mut.Unlock()

	s.failLocked(context, err)
}

func (s *sharedPullerState) failLocked(context string, err error) {
	if s.err != nil {
		return
	}

	l.Infof("Puller (folder %q, file %q): %s: %v", s.folder, s.file.Name, context, err)
	s.err = err
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

func (s *sharedPullerState) copiedFromOrigin() {
	s.mut.Lock()
	s.copyOrigin++
	s.mut.Unlock()
}

func (s *sharedPullerState) pullStarted() {
	s.mut.Lock()
	s.copyTotal--
	s.copyNeeded--
	s.pullTotal++
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

	if s.closed {
		// Already closed
		return false, nil
	}

	if s.pullNeeded+s.copyNeeded != 0 && s.err == nil {
		// Not done yet, and not errored
		return false, nil
	}

	if s.fd != nil {
		if closeErr := s.fd.Close(); closeErr != nil && s.err == nil {
			// This is our error if we weren't errored before. Otherwise we
			// keep the earlier error.
			s.err = closeErr
		}
		s.fd = nil
	}

	s.closed = true

	return true, s.err
}

// Returns the momentarily progress for the puller
func (s *sharedPullerState) Progress() *pullerProgress {
	s.mut.Lock()
	defer s.mut.Unlock()
	total := s.reused + s.copyTotal + s.pullTotal
	done := total - s.copyNeeded - s.pullNeeded
	return &pullerProgress{
		Total:               total,
		Reused:              s.reused,
		CopiedFromOrigin:    s.copyOrigin,
		CopiedFromElsewhere: s.copyTotal - s.copyNeeded - s.copyOrigin,
		Pulled:              s.pullTotal - s.pullNeeded,
		Pulling:             s.pullNeeded,
		BytesTotal:          db.BlocksToSize(total),
		BytesDone:           db.BlocksToSize(done),
	}
}

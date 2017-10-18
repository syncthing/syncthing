// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

// A sharedPullerState is kept for each file that is being synced and is kept
// updated along the way.
type sharedPullerState struct {
	// Immutable, does not require locking
	file        protocol.FileInfo // The new file (desired end state)
	fs          fs.Filesystem
	folder      string
	tempName    string
	realName    string
	reused      int // Number of blocks reused from temporary file
	ignorePerms bool
	hasCurFile  bool              // Whether curFile is set
	curFile     protocol.FileInfo // The file as it exists now in our database
	sparse      bool
	created     time.Time

	// Mutable, must be locked for access
	err               error        // The first error we hit
	fd                fs.File      // The fd of the temp file
	copyTotal         int          // Total number of copy actions for the whole job
	pullTotal         int          // Total number of pull actions for the whole job
	copyOrigin        int          // Number of blocks copied from the original file
	copyOriginShifted int          // Number of blocks copied from the original file but shifted
	copyNeeded        int          // Number of copy actions still pending
	pullNeeded        int          // Number of block pulls still pending
	updated           time.Time    // Time when any of the counters above were last updated
	closed            bool         // True if the file has been finalClosed.
	available         []int32      // Indexes of the blocks that are available in the temporary file
	availableUpdated  time.Time    // Time when list of available blocks was last updated
	mut               sync.RWMutex // Protects the above
}

// A momentary state representing the progress of the puller
type pullerProgress struct {
	Total                   int   `json:"total"`
	Reused                  int   `json:"reused"`
	CopiedFromOrigin        int   `json:"copiedFromOrigin"`
	CopiedFromOriginShifted int   `json:"copiedFromOriginShifted"`
	CopiedFromElsewhere     int   `json:"copiedFromElsewhere"`
	Pulled                  int   `json:"pulled"`
	Pulling                 int   `json:"pulling"`
	BytesDone               int64 `json:"bytesDone"`
	BytesTotal              int64 `json:"bytesTotal"`
}

// A lockedWriterAt synchronizes WriteAt calls with an external mutex.
// WriteAt() is goroutine safe by itself, but not against for example Close().
type lockedWriterAt struct {
	mut *sync.RWMutex
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
	if info, err := s.fs.Stat(dir); err != nil {
		if fs.IsNotExist(err) {
			// XXX: This works around a bug elsewhere, a race condition when
			// things are deleted while being synced. However that happens, we
			// end up with a directory for "foo" with the delete bit, but a
			// file "foo/bar" that we want to sync. We never create the
			// directory, and hence fail to create the file and end up looping
			// forever on it. This breaks that by creating the directory; on
			// next scan it'll be found and the delete bit on it is removed.
			// The user can then clean up as they like...
			l.Infoln("Resurrecting directory", dir)
			if err := s.fs.MkdirAll(dir, 0755); err != nil {
				s.failLocked("resurrect dir", err)
				return nil, err
			}
		} else {
			s.failLocked("dst stat dir", err)
			return nil, err
		}
	} else if info.Mode()&0200 == 0 {
		err := s.fs.Chmod(dir, 0755)
		if !s.ignorePerms && err == nil {
			defer func() {
				err := s.fs.Chmod(dir, info.Mode()&fs.ModePerm)
				if err != nil {
					panic(err)
				}
			}()
		}
	}

	// The permissions to use for the temporary file should be those of the
	// final file, except we need user read & write at minimum. The
	// permissions will be set to the final value later, but in the meantime
	// we don't want to have a temporary file with looser permissions than
	// the final outcome.
	mode := fs.FileMode(s.file.Permissions) | 0600
	if s.ignorePerms {
		// When ignorePerms is set we use a very permissive mode and let the
		// system umask filter it.
		mode = 0666
	}

	// Attempt to create the temp file
	// RDWR because of issue #2994.
	flags := fs.OptReadWrite
	if s.reused == 0 {
		flags |= fs.OptCreate | fs.OptExclusive
	} else if !s.ignorePerms {
		// With sufficiently bad luck when exiting or crashing, we may have
		// had time to chmod the temp file to read only state but not yet
		// moved it to it's final name. This leaves us with a read only temp
		// file that we're going to try to reuse. To handle that, we need to
		// make sure we have write permissions on the file before opening it.
		//
		// When ignorePerms is set we trust that the permissions are fine
		// already and make no modification, as we would otherwise override
		// what the umask dictates.

		if err := s.fs.Chmod(s.tempName, mode); err != nil {
			s.failLocked("dst create chmod", err)
			return nil, err
		}
	}
	fd, err := s.fs.OpenFile(s.tempName, flags, mode)
	if err != nil {
		s.failLocked("dst create", err)
		return nil, err
	}

	// Hide the temporary file
	s.fs.Hide(s.tempName)

	// Don't truncate symlink files, as that will mean that the path will
	// contain a bunch of nulls.
	if s.sparse && !s.file.IsSymlink() {
		// Truncate sets the size of the file. This creates a sparse file or a
		// space reservation, depending on the underlying filesystem.
		if err := fd.Truncate(s.file.Size); err != nil {
			s.failLocked("dst truncate", err)
			return nil, err
		}
	}

	// Same fd will be used by all writers
	s.fd = fd

	return lockedWriterAt{&s.mut, s.fd}, nil
}

// sourceFile opens the existing source file for reading
func (s *sharedPullerState) sourceFile() (fs.File, error) {
	s.mut.Lock()
	defer s.mut.Unlock()

	// If we've already hit an error, return early
	if s.err != nil {
		return nil, s.err
	}

	// Attempt to open the existing file
	fd, err := s.fs.Open(s.realName)
	if err != nil {
		s.failLocked("src open", err)
		return nil, err
	}

	return fd, nil
}

// fail sets the error on the puller state compose of error, and marks the
// sharedPullerState as failed. Is a no-op when called on an already failed state.
func (s *sharedPullerState) fail(context string, err error) {
	s.mut.Lock()
	defer s.mut.Unlock()

	s.failLocked(context, err)
}

func (s *sharedPullerState) failLocked(context string, err error) {
	if s.err != nil || err == nil {
		return
	}

	s.err = fmt.Errorf("%s: %s", context, err.Error())
}

func (s *sharedPullerState) failed() error {
	s.mut.RLock()
	err := s.err
	s.mut.RUnlock()

	return err
}

func (s *sharedPullerState) copyDone(block protocol.BlockInfo) {
	s.mut.Lock()
	s.copyNeeded--
	s.updated = time.Now()
	s.available = append(s.available, int32(block.Offset/protocol.BlockSize))
	s.availableUpdated = time.Now()
	l.Debugln("sharedPullerState", s.folder, s.file.Name, "copyNeeded ->", s.copyNeeded)
	s.mut.Unlock()
}

func (s *sharedPullerState) copiedFromOrigin() {
	s.mut.Lock()
	s.copyOrigin++
	s.updated = time.Now()
	s.mut.Unlock()
}

func (s *sharedPullerState) copiedFromOriginShifted() {
	s.mut.Lock()
	s.copyOrigin++
	s.copyOriginShifted++
	s.updated = time.Now()
	s.mut.Unlock()
}

func (s *sharedPullerState) pullStarted() {
	s.mut.Lock()
	s.copyTotal--
	s.copyNeeded--
	s.pullTotal++
	s.pullNeeded++
	s.updated = time.Now()
	l.Debugln("sharedPullerState", s.folder, s.file.Name, "pullNeeded start ->", s.pullNeeded)
	s.mut.Unlock()
}

func (s *sharedPullerState) pullDone(block protocol.BlockInfo) {
	s.mut.Lock()
	s.pullNeeded--
	s.updated = time.Now()
	s.available = append(s.available, int32(block.Offset/protocol.BlockSize))
	s.availableUpdated = time.Now()
	l.Debugln("sharedPullerState", s.folder, s.file.Name, "pullNeeded done ->", s.pullNeeded)
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
		// This is our error if we weren't errored before. Otherwise we
		// keep the earlier error.
		if fsyncErr := s.fd.Sync(); fsyncErr != nil && s.err == nil {
			s.err = fsyncErr
		}
		if closeErr := s.fd.Close(); closeErr != nil && s.err == nil {
			s.err = closeErr
		}
		s.fd = nil
	}

	s.closed = true

	// Unhide the temporary file when we close it, as it's likely to
	// immediately be renamed to the final name. If this is a failed temp
	// file we will also unhide it, but I'm fine with that as we're now
	// leaving it around for potentially quite a while.
	s.fs.Unhide(s.tempName)

	return true, s.err
}

// Progress returns the momentarily progress for the puller
func (s *sharedPullerState) Progress() *pullerProgress {
	s.mut.RLock()
	defer s.mut.RUnlock()
	total := s.reused + s.copyTotal + s.pullTotal
	done := total - s.copyNeeded - s.pullNeeded
	return &pullerProgress{
		Total:               total,
		Reused:              s.reused,
		CopiedFromOrigin:    s.copyOrigin,
		CopiedFromElsewhere: s.copyTotal - s.copyNeeded - s.copyOrigin,
		Pulled:              s.pullTotal - s.pullNeeded,
		Pulling:             s.pullNeeded,
		BytesTotal:          blocksToSize(total),
		BytesDone:           blocksToSize(done),
	}
}

// Updated returns the time when any of the progress related counters was last updated.
func (s *sharedPullerState) Updated() time.Time {
	s.mut.RLock()
	t := s.updated
	s.mut.RUnlock()
	return t
}

// AvailableUpdated returns the time last time list of available blocks was updated
func (s *sharedPullerState) AvailableUpdated() time.Time {
	s.mut.RLock()
	t := s.availableUpdated
	s.mut.RUnlock()
	return t
}

// Available returns blocks available in the current temporary file
func (s *sharedPullerState) Available() []int32 {
	s.mut.RLock()
	blocks := s.available
	s.mut.RUnlock()
	return blocks
}

func blocksToSize(num int) int64 {
	if num < 2 {
		return protocol.BlockSize / 2
	}
	return int64(num-1)*protocol.BlockSize + protocol.BlockSize/2
}

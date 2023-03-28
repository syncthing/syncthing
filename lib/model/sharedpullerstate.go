// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/osutil"
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
	fsync       bool

	// Mutable, must be locked for access
	err               error           // The first error we hit
	writer            *lockedWriterAt // Wraps fd to prevent fd closing at the same time as writing
	copyTotal         int             // Total number of copy actions for the whole job
	pullTotal         int             // Total number of pull actions for the whole job
	copyOrigin        int             // Number of blocks copied from the original file
	copyOriginShifted int             // Number of blocks copied from the original file but shifted
	copyNeeded        int             // Number of copy actions still pending
	pullNeeded        int             // Number of block pulls still pending
	updated           time.Time       // Time when any of the counters above were last updated
	closed            bool            // True if the file has been finalClosed.
	available         []int           // Indexes of the blocks that are available in the temporary file
	availableUpdated  time.Time       // Time when list of available blocks was last updated
	mut               sync.RWMutex    // Protects the above
}

func newSharedPullerState(file protocol.FileInfo, fs fs.Filesystem, folderID, tempName string, blocks []protocol.BlockInfo, reused []int, ignorePerms, hasCurFile bool, curFile protocol.FileInfo, sparse bool, fsync bool) *sharedPullerState {
	return &sharedPullerState{
		file:             file,
		fs:               fs,
		folder:           folderID,
		tempName:         tempName,
		realName:         file.Name,
		copyTotal:        len(blocks),
		copyNeeded:       len(blocks),
		reused:           len(reused),
		updated:          time.Now(),
		available:        reused,
		availableUpdated: time.Now(),
		ignorePerms:      ignorePerms,
		hasCurFile:       hasCurFile,
		curFile:          curFile,
		mut:              sync.NewRWMutex(),
		sparse:           sparse,
		fsync:            fsync,
		created:          time.Now(),
	}
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

// lockedWriterAt adds a lock to protect from closing the fd at the same time as writing.
// WriteAt() is goroutine safe by itself, but not against for example Close().
type lockedWriterAt struct {
	mut sync.RWMutex
	fd  fs.File
}

// WriteAt itself is goroutine safe, thus just needs to acquire a read-lock to
// prevent closing concurrently (see SyncClose).
func (w *lockedWriterAt) WriteAt(p []byte, off int64) (n int, err error) {
	w.mut.RLock()
	defer w.mut.RUnlock()
	return w.fd.WriteAt(p, off)
}

// SyncClose ensures that no more writes are happening before going ahead and
// syncing and closing the fd, thus needs to acquire a write-lock.
func (w *lockedWriterAt) SyncClose(fsync bool) error {
	w.mut.Lock()
	defer w.mut.Unlock()
	if fsync {
		if err := w.fd.Sync(); err != nil {
			// Sync() is nice if it works but not worth failing the
			// operation over if it fails.
			l.Debugf("fsync failed: %v", err)
		}
	}
	return w.fd.Close()
}

// tempFile returns the fd for the temporary file, reusing an open fd
// or creating the file as necessary.
func (s *sharedPullerState) tempFile() (*lockedWriterAt, error) {
	s.mut.Lock()
	defer s.mut.Unlock()

	// If we've already hit an error, return early
	if s.err != nil {
		return nil, s.err
	}

	// If the temp file is already open, return the file descriptor
	if s.writer != nil {
		return s.writer, nil
	}

	if err := s.addWriterLocked(); err != nil {
		s.failLocked(err)
		return nil, err
	}

	return s.writer, nil
}

func (s *sharedPullerState) addWriterLocked() error {
	return inWritableDir(s.tempFileInWritableDir, s.fs, s.tempName, s.ignorePerms)
}

// tempFileInWritableDir should only be called from tempFile.
func (s *sharedPullerState) tempFileInWritableDir(_ string) error {
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
		// moved it to its final name. This leaves us with a read only temp
		// file that we're going to try to reuse. To handle that, we need to
		// make sure we have write permissions on the file before opening it.
		//
		// When ignorePerms is set we trust that the permissions are fine
		// already and make no modification, as we would otherwise override
		// what the umask dictates.

		if err := s.fs.Chmod(s.tempName, mode); err != nil {
			return fmt.Errorf("setting perms on temp file: %w", err)
		}
	}
	fd, err := s.fs.OpenFile(s.tempName, flags, mode)
	if err != nil {
		return fmt.Errorf("opening temp file: %w", err)
	}

	// Hide the temporary file
	s.fs.Hide(s.tempName)

	// Don't truncate symlink files, as that will mean that the path will
	// contain a bunch of nulls.
	if s.sparse && !s.file.IsSymlink() {
		size := s.file.Size
		// Trailer added to encrypted files
		if len(s.file.Encrypted) > 0 {
			size += encryptionTrailerSize(s.file)
		}
		// Truncate sets the size of the file. This creates a sparse file or a
		// space reservation, depending on the underlying filesystem.
		if err := fd.Truncate(size); err != nil {
			// The truncate call failed. That can happen in some cases when
			// space reservation isn't possible or over some network
			// filesystems... This generally doesn't matter.

			if s.reused > 0 {
				// ... but if we are attempting to reuse a file we have a
				// corner case when the old file is larger than the new one
				// and we can't just overwrite blocks and let the old data
				// linger at the end. In this case we attempt a delete of
				// the file and hope for better luck next time, when we
				// should come around with s.reused == 0.

				fd.Close()

				if remErr := s.fs.Remove(s.tempName); remErr != nil {
					l.Debugln("failed to remove temporary file:", remErr)
				}

				return err
			}
		}
	}

	// Same fd will be used by all writers
	s.writer = &lockedWriterAt{sync.NewRWMutex(), fd}
	return nil
}

// fail sets the error on the puller state compose of error, and marks the
// sharedPullerState as failed. Is a no-op when called on an already failed state.
func (s *sharedPullerState) fail(err error) {
	s.mut.Lock()
	defer s.mut.Unlock()

	s.failLocked(err)
}

func (s *sharedPullerState) failLocked(err error) {
	if s.err != nil || err == nil {
		return
	}

	s.err = err
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
	s.available = append(s.available, int(block.Offset/int64(s.file.BlockSize())))
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
	s.available = append(s.available, int(block.Offset/int64(s.file.BlockSize())))
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

	if len(s.file.Encrypted) > 0 {
		if err := s.finalizeEncrypted(); err != nil && s.err == nil {
			// This is our error as we weren't errored before.
			s.err = err
		}
	}

	if s.writer != nil {
		if err := s.writer.SyncClose(s.fsync); err != nil && s.err == nil {
			// This is our error as we weren't errored before.
			s.err = err
		}
		s.writer = nil
	}

	s.closed = true

	// Unhide the temporary file when we close it, as it's likely to
	// immediately be renamed to the final name. If this is a failed temp
	// file we will also unhide it, but I'm fine with that as we're now
	// leaving it around for potentially quite a while.
	s.fs.Unhide(s.tempName)

	return true, s.err
}

// finalizeEncrypted adds a trailer to the encrypted file containing the
// serialized FileInfo and the length of that FileInfo. When initializing a
// folder from encrypted data we can extract this FileInfo from the end of
// the file and regain the original metadata.
func (s *sharedPullerState) finalizeEncrypted() error {
	if s.writer == nil {
		if err := s.addWriterLocked(); err != nil {
			return err
		}
	}
	trailerSize, err := writeEncryptionTrailer(s.file, s.writer)
	if err != nil {
		return err
	}
	s.file.Size += trailerSize
	s.file.EncryptionTrailerSize = int(trailerSize)
	return nil
}

// Returns the size of the written trailer.
func writeEncryptionTrailer(file protocol.FileInfo, writer io.WriterAt) (int64, error) {
	// Here the file is in native format, while encryption happens in
	// wire format (always slashes).
	wireFile := file
	wireFile.Name = osutil.NormalizedFilename(wireFile.Name)

	trailerSize := encryptionTrailerSize(wireFile)
	bs := make([]byte, trailerSize)
	n, err := wireFile.MarshalTo(bs)
	if err != nil {
		return 0, err
	}
	binary.BigEndian.PutUint32(bs[n:], uint32(n))
	bs = bs[:n+4]

	if _, err := writer.WriteAt(bs, wireFile.Size); err != nil {
		return 0, err
	}

	return trailerSize, nil
}

func encryptionTrailerSize(file protocol.FileInfo) int64 {
	return int64(file.ProtoSize()) + 4
}

// Progress returns the momentarily progress for the puller
func (s *sharedPullerState) Progress() *pullerProgress {
	s.mut.RLock()
	defer s.mut.RUnlock()
	total := s.reused + s.copyTotal + s.pullTotal
	done := total - s.copyNeeded - s.pullNeeded
	file := len(s.file.Blocks)
	return &pullerProgress{
		Total:               total,
		Reused:              s.reused,
		CopiedFromOrigin:    s.copyOrigin,
		CopiedFromElsewhere: s.copyTotal - s.copyNeeded - s.copyOrigin,
		Pulled:              s.pullTotal - s.pullNeeded,
		Pulling:             s.pullNeeded,
		BytesTotal:          blocksToSize(total, file, s.file.BlockSize(), s.file.Size),
		BytesDone:           blocksToSize(done, file, s.file.BlockSize(), s.file.Size),
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
func (s *sharedPullerState) Available() []int {
	s.mut.RLock()
	blocks := s.available
	s.mut.RUnlock()
	return blocks
}

func blocksToSize(blocks, blocksInFile, blockSize int, fileSize int64) int64 {
	// The last/only block has somewhere between 1 and blockSize bytes. We do
	// not know whether the smaller block is part of the blocks and use an
	// estimate assuming a random chance that the small block is contained.
	if blocksInFile == 0 {
		return 0
	}
	return int64(blocks)*int64(blockSize) - (int64(blockSize)-fileSize%int64(blockSize))*int64(blocks)/int64(blocksInFile)
}

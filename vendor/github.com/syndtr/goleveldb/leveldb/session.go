// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/journal"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

// ErrManifestCorrupted records manifest corruption. This error will be
// wrapped with errors.ErrCorrupted.
type ErrManifestCorrupted struct {
	Field  string
	Reason string
}

func (e *ErrManifestCorrupted) Error() string {
	return fmt.Sprintf("leveldb: manifest corrupted (field '%s'): %s", e.Field, e.Reason)
}

func newErrManifestCorrupted(fd storage.FileDesc, field, reason string) error {
	return errors.NewErrCorrupted(fd, &ErrManifestCorrupted{field, reason})
}

// session represent a persistent database session.
type session struct {
	// Need 64-bit alignment.
	stNextFileNum    int64 // current unused file number
	stJournalNum     int64 // current journal file number; need external synchronization
	stPrevJournalNum int64 // prev journal file number; no longer used; for compatibility with older version of leveldb
	stTempFileNum    int64
	stSeqNum         uint64 // last mem compacted seq; need external synchronization

	stor     storage.Storage
	storLock storage.Locker
	o        *cachedOptions
	icmp     *iComparer
	tops     *tOps
	fileRef  map[int64]int

	manifest       *journal.Writer
	manifestWriter storage.Writer
	manifestFd     storage.FileDesc

	stCompPtrs []internalKey // compaction pointers; need external synchronization
	stVersion  *version      // current version
	vmu        sync.Mutex
}

// Creates new initialized session instance.
func newSession(stor storage.Storage, o *opt.Options) (s *session, err error) {
	if stor == nil {
		return nil, os.ErrInvalid
	}
	storLock, err := stor.Lock()
	if err != nil {
		return
	}
	s = &session{
		stor:     stor,
		storLock: storLock,
		fileRef:  make(map[int64]int),
	}
	s.setOptions(o)
	s.tops = newTableOps(s)
	s.setVersion(newVersion(s))
	s.log("log@legend F·NumFile S·FileSize N·Entry C·BadEntry B·BadBlock Ke·KeyError D·DroppedEntry L·Level Q·SeqNum T·TimeElapsed")
	return
}

// Close session.
func (s *session) close() {
	s.tops.close()
	if s.manifest != nil {
		s.manifest.Close()
	}
	if s.manifestWriter != nil {
		s.manifestWriter.Close()
	}
	s.manifest = nil
	s.manifestWriter = nil
	s.setVersion(&version{s: s, closing: true})
}

// Release session lock.
func (s *session) release() {
	s.storLock.Unlock()
}

// Create a new database session; need external synchronization.
func (s *session) create() error {
	// create manifest
	return s.newManifest(nil, nil)
}

// Recover a database session; need external synchronization.
func (s *session) recover() (err error) {
	defer func() {
		if os.IsNotExist(err) {
			// Don't return os.ErrNotExist if the underlying storage contains
			// other files that belong to LevelDB. So the DB won't get trashed.
			if fds, _ := s.stor.List(storage.TypeAll); len(fds) > 0 {
				err = &errors.ErrCorrupted{Fd: storage.FileDesc{Type: storage.TypeManifest}, Err: &errors.ErrMissingFiles{}}
			}
		}
	}()

	fd, err := s.stor.GetMeta()
	if err != nil {
		return
	}

	reader, err := s.stor.Open(fd)
	if err != nil {
		return
	}
	defer reader.Close()

	var (
		// Options.
		strict = s.o.GetStrict(opt.StrictManifest)

		jr      = journal.NewReader(reader, dropper{s, fd}, strict, true)
		rec     = &sessionRecord{}
		staging = s.stVersion.newStaging()
	)
	for {
		var r io.Reader
		r, err = jr.Next()
		if err != nil {
			if err == io.EOF {
				err = nil
				break
			}
			return errors.SetFd(err, fd)
		}

		err = rec.decode(r)
		if err == nil {
			// save compact pointers
			for _, r := range rec.compPtrs {
				s.setCompPtr(r.level, internalKey(r.ikey))
			}
			// commit record to version staging
			staging.commit(rec)
		} else {
			err = errors.SetFd(err, fd)
			if strict || !errors.IsCorrupted(err) {
				return
			}
			s.logf("manifest error: %v (skipped)", errors.SetFd(err, fd))
		}
		rec.resetCompPtrs()
		rec.resetAddedTables()
		rec.resetDeletedTables()
	}

	switch {
	case !rec.has(recComparer):
		return newErrManifestCorrupted(fd, "comparer", "missing")
	case rec.comparer != s.icmp.uName():
		return newErrManifestCorrupted(fd, "comparer", fmt.Sprintf("mismatch: want '%s', got '%s'", s.icmp.uName(), rec.comparer))
	case !rec.has(recNextFileNum):
		return newErrManifestCorrupted(fd, "next-file-num", "missing")
	case !rec.has(recJournalNum):
		return newErrManifestCorrupted(fd, "journal-file-num", "missing")
	case !rec.has(recSeqNum):
		return newErrManifestCorrupted(fd, "seq-num", "missing")
	}

	s.manifestFd = fd
	s.setVersion(staging.finish())
	s.setNextFileNum(rec.nextFileNum)
	s.recordCommited(rec)
	return nil
}

// Commit session; need external synchronization.
func (s *session) commit(r *sessionRecord) (err error) {
	v := s.version()
	defer v.release()

	// spawn new version based on current version
	nv := v.spawn(r)

	if s.manifest == nil {
		// manifest journal writer not yet created, create one
		err = s.newManifest(r, nv)
	} else {
		err = s.flushManifest(r)
	}

	// finally, apply new version if no error rise
	if err == nil {
		s.setVersion(nv)
	}

	return
}

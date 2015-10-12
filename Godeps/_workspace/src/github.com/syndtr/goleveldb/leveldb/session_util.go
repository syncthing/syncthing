// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"fmt"
	"sync/atomic"

	"github.com/syndtr/goleveldb/leveldb/journal"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

// Logging.

type dropper struct {
	s    *session
	file storage.File
}

func (d dropper) Drop(err error) {
	if e, ok := err.(*journal.ErrCorrupted); ok {
		d.s.logf("journal@drop %s-%d S·%s %q", d.file.Type(), d.file.Num(), shortenb(e.Size), e.Reason)
	} else {
		d.s.logf("journal@drop %s-%d %q", d.file.Type(), d.file.Num(), err)
	}
}

func (s *session) log(v ...interface{})                 { s.stor.Log(fmt.Sprint(v...)) }
func (s *session) logf(format string, v ...interface{}) { s.stor.Log(fmt.Sprintf(format, v...)) }

// File utils.

func (s *session) getJournalFile(num uint64) storage.File {
	return s.stor.GetFile(num, storage.TypeJournal)
}

func (s *session) getTableFile(num uint64) storage.File {
	return s.stor.GetFile(num, storage.TypeTable)
}

func (s *session) getFiles(t storage.FileType) ([]storage.File, error) {
	return s.stor.GetFiles(t)
}

func (s *session) newTemp() storage.File {
	num := atomic.AddUint64(&s.stTempFileNum, 1) - 1
	return s.stor.GetFile(num, storage.TypeTemp)
}

func (s *session) tableFileFromRecord(r atRecord) *tFile {
	return newTableFile(s.getTableFile(r.num), r.size, r.imin, r.imax)
}

// Session state.

// Get current version. This will incr version ref, must call
// version.release (exactly once) after use.
func (s *session) version() *version {
	s.vmu.Lock()
	defer s.vmu.Unlock()
	s.stVersion.ref++
	return s.stVersion
}

// Set current version to v.
func (s *session) setVersion(v *version) {
	s.vmu.Lock()
	v.ref = 1 // Holds by session.
	if old := s.stVersion; old != nil {
		v.ref++ // Holds by old version.
		old.next = v
		old.releaseNB()
	}
	s.stVersion = v
	s.vmu.Unlock()
}

// Get current unused file number.
func (s *session) nextFileNum() uint64 {
	return atomic.LoadUint64(&s.stNextFileNum)
}

// Set current unused file number to num.
func (s *session) setNextFileNum(num uint64) {
	atomic.StoreUint64(&s.stNextFileNum, num)
}

// Mark file number as used.
func (s *session) markFileNum(num uint64) {
	nextFileNum := num + 1
	for {
		old, x := s.stNextFileNum, nextFileNum
		if old > x {
			x = old
		}
		if atomic.CompareAndSwapUint64(&s.stNextFileNum, old, x) {
			break
		}
	}
}

// Allocate a file number.
func (s *session) allocFileNum() uint64 {
	return atomic.AddUint64(&s.stNextFileNum, 1) - 1
}

// Reuse given file number.
func (s *session) reuseFileNum(num uint64) {
	for {
		old, x := s.stNextFileNum, num
		if old != x+1 {
			x = old
		}
		if atomic.CompareAndSwapUint64(&s.stNextFileNum, old, x) {
			break
		}
	}
}

// Manifest related utils.

// Fill given session record obj with current states; need external
// synchronization.
func (s *session) fillRecord(r *sessionRecord, snapshot bool) {
	r.setNextFileNum(s.nextFileNum())

	if snapshot {
		if !r.has(recJournalNum) {
			r.setJournalNum(s.stJournalNum)
		}

		if !r.has(recSeqNum) {
			r.setSeqNum(s.stSeqNum)
		}

		for level, ik := range s.stCompPtrs {
			if ik != nil {
				r.addCompPtr(level, ik)
			}
		}

		r.setComparer(s.icmp.uName())
	}
}

// Mark if record has been committed, this will update session state;
// need external synchronization.
func (s *session) recordCommited(r *sessionRecord) {
	if r.has(recJournalNum) {
		s.stJournalNum = r.journalNum
	}

	if r.has(recPrevJournalNum) {
		s.stPrevJournalNum = r.prevJournalNum
	}

	if r.has(recSeqNum) {
		s.stSeqNum = r.seqNum
	}

	for _, p := range r.compPtrs {
		s.stCompPtrs[p.level] = iKey(p.ikey)
	}
}

// Create a new manifest file; need external synchronization.
func (s *session) newManifest(rec *sessionRecord, v *version) (err error) {
	num := s.allocFileNum()
	file := s.stor.GetFile(num, storage.TypeManifest)
	writer, err := file.Create()
	if err != nil {
		return
	}
	jw := journal.NewWriter(writer)

	if v == nil {
		v = s.version()
		defer v.release()
	}
	if rec == nil {
		rec = &sessionRecord{}
	}
	s.fillRecord(rec, true)
	v.fillRecord(rec)

	defer func() {
		if err == nil {
			s.recordCommited(rec)
			if s.manifest != nil {
				s.manifest.Close()
			}
			if s.manifestWriter != nil {
				s.manifestWriter.Close()
			}
			if s.manifestFile != nil {
				s.manifestFile.Remove()
			}
			s.manifestFile = file
			s.manifestWriter = writer
			s.manifest = jw
		} else {
			writer.Close()
			file.Remove()
			s.reuseFileNum(num)
		}
	}()

	w, err := jw.Next()
	if err != nil {
		return
	}
	err = rec.encode(w)
	if err != nil {
		return
	}
	err = jw.Flush()
	if err != nil {
		return
	}
	err = s.stor.SetManifest(file)
	return
}

// Flush record to disk.
func (s *session) flushManifest(rec *sessionRecord) (err error) {
	s.fillRecord(rec, false)
	w, err := s.manifest.Next()
	if err != nil {
		return
	}
	err = rec.encode(w)
	if err != nil {
		return
	}
	err = s.manifest.Flush()
	if err != nil {
		return
	}
	if !s.o.GetNoSync() {
		err = s.manifestWriter.Sync()
		if err != nil {
			return
		}
	}
	s.recordCommited(rec)
	return
}

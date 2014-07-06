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

// logging

type dropper struct {
	s    *session
	file storage.File
}

func (d dropper) Drop(err error) {
	if e, ok := err.(journal.DroppedError); ok {
		d.s.logf("journal@drop %s-%d S·%s %q", d.file.Type(), d.file.Num(), shortenb(e.Size), e.Reason)
	} else {
		d.s.logf("journal@drop %s-%d %q", d.file.Type(), d.file.Num(), err)
	}
}

func (s *session) log(v ...interface{}) {
	s.stor.Log(fmt.Sprint(v...))
}

func (s *session) logf(format string, v ...interface{}) {
	s.stor.Log(fmt.Sprintf(format, v...))
}

// file utils

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

// session state

// Get current version.
func (s *session) version() *version {
	s.vmu.Lock()
	defer s.vmu.Unlock()
	s.stVersion.ref++
	return s.stVersion
}

// Get current version; no barrier.
func (s *session) version_NB() *version {
	return s.stVersion
}

// Set current version to v.
func (s *session) setVersion(v *version) {
	s.vmu.Lock()
	v.ref = 1
	if old := s.stVersion; old != nil {
		v.ref++
		old.next = v
		old.release_NB()
	}
	s.stVersion = v
	s.vmu.Unlock()
}

// Get current unused file number.
func (s *session) fileNum() uint64 {
	return atomic.LoadUint64(&s.stFileNum)
}

// Get current unused file number to num.
func (s *session) setFileNum(num uint64) {
	atomic.StoreUint64(&s.stFileNum, num)
}

// Mark file number as used.
func (s *session) markFileNum(num uint64) {
	num += 1
	for {
		old, x := s.stFileNum, num
		if old > x {
			x = old
		}
		if atomic.CompareAndSwapUint64(&s.stFileNum, old, x) {
			break
		}
	}
}

// Allocate a file number.
func (s *session) allocFileNum() (num uint64) {
	return atomic.AddUint64(&s.stFileNum, 1) - 1
}

// Reuse given file number.
func (s *session) reuseFileNum(num uint64) {
	for {
		old, x := s.stFileNum, num
		if old != x+1 {
			x = old
		}
		if atomic.CompareAndSwapUint64(&s.stFileNum, old, x) {
			break
		}
	}
}

// manifest related utils

// Fill given session record obj with current states; need external
// synchronization.
func (s *session) fillRecord(r *sessionRecord, snapshot bool) {
	r.setNextNum(s.fileNum())

	if snapshot {
		if !r.has(recJournalNum) {
			r.setJournalNum(s.stJournalNum)
		}

		if !r.has(recSeq) {
			r.setSeq(s.stSeq)
		}

		for level, ik := range s.stCPtrs {
			if ik != nil {
				r.addCompactionPointer(level, ik)
			}
		}

		r.setComparer(s.icmp.uName())
	}
}

// Mark if record has been commited, this will update session state;
// need external synchronization.
func (s *session) recordCommited(r *sessionRecord) {
	if r.has(recJournalNum) {
		s.stJournalNum = r.journalNum
	}

	if r.has(recPrevJournalNum) {
		s.stPrevJournalNum = r.prevJournalNum
	}

	if r.has(recSeq) {
		s.stSeq = r.seq
	}

	for _, p := range r.compactionPointers {
		s.stCPtrs[p.level] = iKey(p.key)
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
		v = s.version_NB()
	}
	if rec == nil {
		rec = new(sessionRecord)
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
	err = s.manifestWriter.Sync()
	if err != nil {
		return
	}
	s.recordCommited(rec)
	return
}

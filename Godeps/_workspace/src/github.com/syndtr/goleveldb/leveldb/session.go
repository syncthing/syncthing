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
	"sync/atomic"

	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/journal"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type ErrManifestCorrupted struct {
	Field  string
	Reason string
}

func (e *ErrManifestCorrupted) Error() string {
	return fmt.Sprintf("leveldb: manifest corrupted (field '%s'): %s", e.Field, e.Reason)
}

func newErrManifestCorrupted(f storage.File, field, reason string) error {
	return errors.NewErrCorrupted(f, &ErrManifestCorrupted{field, reason})
}

// session represent a persistent database session.
type session struct {
	// Need 64-bit alignment.
	stNextFileNum    uint64 // current unused file number
	stJournalNum     uint64 // current journal file number; need external synchronization
	stPrevJournalNum uint64 // prev journal file number; no longer used; for compatibility with older version of leveldb
	stSeqNum         uint64 // last mem compacted seq; need external synchronization
	stTempFileNum    uint64

	stor     storage.Storage
	storLock util.Releaser
	o        *cachedOptions
	icmp     *iComparer
	tops     *tOps

	manifest       *journal.Writer
	manifestWriter storage.Writer
	manifestFile   storage.File

	stCompPtrs []iKey   // compaction pointers; need external synchronization
	stVersion  *version // current version
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
		stor:       stor,
		storLock:   storLock,
		stCompPtrs: make([]iKey, o.GetNumLevel()),
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
	s.manifestFile = nil
	s.stVersion = nil
}

// Release session lock.
func (s *session) release() {
	s.storLock.Release()
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
			if files, _ := s.stor.GetFiles(storage.TypeAll); len(files) > 0 {
				err = &errors.ErrCorrupted{File: &storage.FileInfo{Type: storage.TypeManifest}, Err: &errors.ErrMissingFiles{}}
			}
		}
	}()

	m, err := s.stor.GetManifest()
	if err != nil {
		return
	}

	reader, err := m.Open()
	if err != nil {
		return
	}
	defer reader.Close()
	strict := s.o.GetStrict(opt.StrictManifest)
	jr := journal.NewReader(reader, dropper{s, m}, strict, true)

	staging := s.stVersion.newStaging()
	rec := &sessionRecord{numLevel: s.o.GetNumLevel()}
	for {
		var r io.Reader
		r, err = jr.Next()
		if err != nil {
			if err == io.EOF {
				err = nil
				break
			}
			return errors.SetFile(err, m)
		}

		err = rec.decode(r)
		if err == nil {
			// save compact pointers
			for _, r := range rec.compPtrs {
				s.stCompPtrs[r.level] = iKey(r.ikey)
			}
			// commit record to version staging
			staging.commit(rec)
		} else {
			err = errors.SetFile(err, m)
			if strict || !errors.IsCorrupted(err) {
				return
			} else {
				s.logf("manifest error: %v (skipped)", errors.SetFile(err, m))
			}
		}
		rec.resetCompPtrs()
		rec.resetAddedTables()
		rec.resetDeletedTables()
	}

	switch {
	case !rec.has(recComparer):
		return newErrManifestCorrupted(m, "comparer", "missing")
	case rec.comparer != s.icmp.uName():
		return newErrManifestCorrupted(m, "comparer", fmt.Sprintf("mismatch: want '%s', got '%s'", s.icmp.uName(), rec.comparer))
	case !rec.has(recNextFileNum):
		return newErrManifestCorrupted(m, "next-file-num", "missing")
	case !rec.has(recJournalNum):
		return newErrManifestCorrupted(m, "journal-file-num", "missing")
	case !rec.has(recSeqNum):
		return newErrManifestCorrupted(m, "seq-num", "missing")
	}

	s.manifestFile = m
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

// Pick a compaction based on current state; need external synchronization.
func (s *session) pickCompaction() *compaction {
	v := s.version()

	var level int
	var t0 tFiles
	if v.cScore >= 1 {
		level = v.cLevel
		cptr := s.stCompPtrs[level]
		tables := v.tables[level]
		for _, t := range tables {
			if cptr == nil || s.icmp.Compare(t.imax, cptr) > 0 {
				t0 = append(t0, t)
				break
			}
		}
		if len(t0) == 0 {
			t0 = append(t0, tables[0])
		}
	} else {
		if p := atomic.LoadPointer(&v.cSeek); p != nil {
			ts := (*tSet)(p)
			level = ts.level
			t0 = append(t0, ts.table)
		} else {
			v.release()
			return nil
		}
	}

	return newCompaction(s, v, level, t0)
}

// Create compaction from given level and range; need external synchronization.
func (s *session) getCompactionRange(level int, umin, umax []byte) *compaction {
	v := s.version()

	t0 := v.tables[level].getOverlaps(nil, s.icmp, umin, umax, level == 0)
	if len(t0) == 0 {
		v.release()
		return nil
	}

	// Avoid compacting too much in one shot in case the range is large.
	// But we cannot do this for level-0 since level-0 files can overlap
	// and we must not pick one file and drop another older file if the
	// two files overlap.
	if level > 0 {
		limit := uint64(v.s.o.GetCompactionSourceLimit(level))
		total := uint64(0)
		for i, t := range t0 {
			total += t.size
			if total >= limit {
				s.logf("table@compaction limiting F·%d -> F·%d", len(t0), i+1)
				t0 = t0[:i+1]
				break
			}
		}
	}

	return newCompaction(s, v, level, t0)
}

func newCompaction(s *session, v *version, level int, t0 tFiles) *compaction {
	c := &compaction{
		s:             s,
		v:             v,
		level:         level,
		tables:        [2]tFiles{t0, nil},
		maxGPOverlaps: uint64(s.o.GetCompactionGPOverlaps(level)),
		tPtrs:         make([]int, s.o.GetNumLevel()),
	}
	c.expand()
	c.save()
	return c
}

// compaction represent a compaction state.
type compaction struct {
	s *session
	v *version

	level         int
	tables        [2]tFiles
	maxGPOverlaps uint64

	gp                tFiles
	gpi               int
	seenKey           bool
	gpOverlappedBytes uint64
	imin, imax        iKey
	tPtrs             []int
	released          bool

	snapGPI               int
	snapSeenKey           bool
	snapGPOverlappedBytes uint64
	snapTPtrs             []int
}

func (c *compaction) save() {
	c.snapGPI = c.gpi
	c.snapSeenKey = c.seenKey
	c.snapGPOverlappedBytes = c.gpOverlappedBytes
	c.snapTPtrs = append(c.snapTPtrs[:0], c.tPtrs...)
}

func (c *compaction) restore() {
	c.gpi = c.snapGPI
	c.seenKey = c.snapSeenKey
	c.gpOverlappedBytes = c.snapGPOverlappedBytes
	c.tPtrs = append(c.tPtrs[:0], c.snapTPtrs...)
}

func (c *compaction) release() {
	if !c.released {
		c.released = true
		c.v.release()
	}
}

// Expand compacted tables; need external synchronization.
func (c *compaction) expand() {
	limit := uint64(c.s.o.GetCompactionExpandLimit(c.level))
	vt0, vt1 := c.v.tables[c.level], c.v.tables[c.level+1]

	t0, t1 := c.tables[0], c.tables[1]
	imin, imax := t0.getRange(c.s.icmp)
	// We expand t0 here just incase ukey hop across tables.
	t0 = vt0.getOverlaps(t0, c.s.icmp, imin.ukey(), imax.ukey(), c.level == 0)
	if len(t0) != len(c.tables[0]) {
		imin, imax = t0.getRange(c.s.icmp)
	}
	t1 = vt1.getOverlaps(t1, c.s.icmp, imin.ukey(), imax.ukey(), false)
	// Get entire range covered by compaction.
	amin, amax := append(t0, t1...).getRange(c.s.icmp)

	// See if we can grow the number of inputs in "level" without
	// changing the number of "level+1" files we pick up.
	if len(t1) > 0 {
		exp0 := vt0.getOverlaps(nil, c.s.icmp, amin.ukey(), amax.ukey(), c.level == 0)
		if len(exp0) > len(t0) && t1.size()+exp0.size() < limit {
			xmin, xmax := exp0.getRange(c.s.icmp)
			exp1 := vt1.getOverlaps(nil, c.s.icmp, xmin.ukey(), xmax.ukey(), false)
			if len(exp1) == len(t1) {
				c.s.logf("table@compaction expanding L%d+L%d (F·%d S·%s)+(F·%d S·%s) -> (F·%d S·%s)+(F·%d S·%s)",
					c.level, c.level+1, len(t0), shortenb(int(t0.size())), len(t1), shortenb(int(t1.size())),
					len(exp0), shortenb(int(exp0.size())), len(exp1), shortenb(int(exp1.size())))
				imin, imax = xmin, xmax
				t0, t1 = exp0, exp1
				amin, amax = append(t0, t1...).getRange(c.s.icmp)
			}
		}
	}

	// Compute the set of grandparent files that overlap this compaction
	// (parent == level+1; grandparent == level+2)
	if c.level+2 < c.s.o.GetNumLevel() {
		c.gp = c.v.tables[c.level+2].getOverlaps(c.gp, c.s.icmp, amin.ukey(), amax.ukey(), false)
	}

	c.tables[0], c.tables[1] = t0, t1
	c.imin, c.imax = imin, imax
}

// Check whether compaction is trivial.
func (c *compaction) trivial() bool {
	return len(c.tables[0]) == 1 && len(c.tables[1]) == 0 && c.gp.size() <= c.maxGPOverlaps
}

func (c *compaction) baseLevelForKey(ukey []byte) bool {
	for level, tables := range c.v.tables[c.level+2:] {
		for c.tPtrs[level] < len(tables) {
			t := tables[c.tPtrs[level]]
			if c.s.icmp.uCompare(ukey, t.imax.ukey()) <= 0 {
				// We've advanced far enough.
				if c.s.icmp.uCompare(ukey, t.imin.ukey()) >= 0 {
					// Key falls in this file's range, so definitely not base level.
					return false
				}
				break
			}
			c.tPtrs[level]++
		}
	}
	return true
}

func (c *compaction) shouldStopBefore(ikey iKey) bool {
	for ; c.gpi < len(c.gp); c.gpi++ {
		gp := c.gp[c.gpi]
		if c.s.icmp.Compare(ikey, gp.imax) <= 0 {
			break
		}
		if c.seenKey {
			c.gpOverlappedBytes += gp.size
		}
	}
	c.seenKey = true

	if c.gpOverlappedBytes > c.maxGPOverlaps {
		// Too much overlap for current output; start new output.
		c.gpOverlappedBytes = 0
		return true
	}
	return false
}

// Creates an iterator.
func (c *compaction) newIterator() iterator.Iterator {
	// Creates iterator slice.
	icap := len(c.tables)
	if c.level == 0 {
		// Special case for level-0
		icap = len(c.tables[0]) + 1
	}
	its := make([]iterator.Iterator, 0, icap)

	// Options.
	ro := &opt.ReadOptions{
		DontFillCache: true,
		Strict:        opt.StrictOverride,
	}
	strict := c.s.o.GetStrict(opt.StrictCompaction)
	if strict {
		ro.Strict |= opt.StrictReader
	}

	for i, tables := range c.tables {
		if len(tables) == 0 {
			continue
		}

		// Level-0 is not sorted and may overlaps each other.
		if c.level+i == 0 {
			for _, t := range tables {
				its = append(its, c.s.tops.newIterator(t, nil, ro))
			}
		} else {
			it := iterator.NewIndexedIterator(tables.newIndexIterator(c.s.tops, c.s.icmp, nil, ro), strict)
			its = append(its, it)
		}
	}

	return iterator.NewMergedIterator(its, c.s.icmp, strict)
}

// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"

	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/journal"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// session represent a persistent database session.
type session struct {
	// Need 64-bit alignment.
	stFileNum        uint64 // current unused file number
	stJournalNum     uint64 // current journal file number; need external synchronization
	stPrevJournalNum uint64 // prev journal file number; no longer used; for compatibility with older version of leveldb
	stSeq            uint64 // last mem compacted seq; need external synchronization
	stTempFileNum    uint64

	stor     storage.Storage
	storLock util.Releaser
	o        *opt.Options
	icmp     *iComparer
	tops     *tOps

	manifest       *journal.Writer
	manifestWriter storage.Writer
	manifestFile   storage.File

	stCptrs   [kNumLevels]iKey // compact pointers; need external synchronization
	stVersion *version         // current version
	vmu       sync.Mutex
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
	}
	s.setOptions(o)
	s.tops = newTableOps(s, s.o.GetMaxOpenFiles())
	s.setVersion(&version{s: s})
	s.log("log@legend F·NumFile S·FileSize N·Entry C·BadEntry B·BadBlock D·DeletedEntry L·Level Q·SeqNum T·TimeElapsed")
	return
}

// Close session.
func (s *session) close() {
	s.tops.close()
	if bc := s.o.GetBlockCache(); bc != nil {
		bc.Purge(nil)
	}
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
				err = ErrCorrupted{Type: CorruptedManifest, Err: errors.New("leveldb: manifest file missing")}
			}
		}
	}()

	file, err := s.stor.GetManifest()
	if err != nil {
		return
	}

	reader, err := file.Open()
	if err != nil {
		return
	}
	defer reader.Close()
	strict := s.o.GetStrict(opt.StrictManifest)
	jr := journal.NewReader(reader, dropper{s, file}, strict, true)

	staging := s.version_NB().newStaging()
	rec := &sessionRecord{}
	for {
		var r io.Reader
		r, err = jr.Next()
		if err != nil {
			if err == io.EOF {
				err = nil
				break
			}
			return
		}

		err = rec.decode(r)
		if err == nil {
			// save compact pointers
			for _, r := range rec.compactionPointers {
				s.stCptrs[r.level] = iKey(r.ikey)
			}
			// commit record to version staging
			staging.commit(rec)
		} else if strict {
			return ErrCorrupted{Type: CorruptedManifest, Err: err}
		} else {
			s.logf("manifest error: %v (skipped)", err)
		}
		rec.resetCompactionPointers()
		rec.resetAddedTables()
		rec.resetDeletedTables()
	}

	switch {
	case !rec.has(recComparer):
		return ErrCorrupted{Type: CorruptedManifest, Err: errors.New("leveldb: manifest missing comparer name")}
	case rec.comparer != s.icmp.uName():
		return ErrCorrupted{Type: CorruptedManifest, Err: errors.New("leveldb: comparer mismatch, " + "want '" + s.icmp.uName() + "', " + "got '" + rec.comparer + "'")}
	case !rec.has(recNextNum):
		return ErrCorrupted{Type: CorruptedManifest, Err: errors.New("leveldb: manifest missing next file number")}
	case !rec.has(recJournalNum):
		return ErrCorrupted{Type: CorruptedManifest, Err: errors.New("leveldb: manifest missing journal file number")}
	case !rec.has(recSeq):
		return ErrCorrupted{Type: CorruptedManifest, Err: errors.New("leveldb: manifest missing seq number")}
	}

	s.manifestFile = file
	s.setVersion(staging.finish())
	s.setFileNum(rec.nextNum)
	s.recordCommited(rec)
	return nil
}

// Commit session; need external synchronization.
func (s *session) commit(r *sessionRecord) (err error) {
	// spawn new version based on current version
	nv := s.version_NB().spawn(r)

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
	v := s.version_NB()

	var level int
	var t0 tFiles
	if v.cScore >= 1 {
		level = v.cLevel
		cptr := s.stCptrs[level]
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
			return nil
		}
	}

	c := &compaction{s: s, v: v, level: level}
	if level == 0 {
		imin, imax := t0.getRange(s.icmp)
		t0 = v.tables[0].getOverlaps(t0[:0], s.icmp, imin.ukey(), imax.ukey(), true)
	}

	c.tables[0] = t0
	c.expand()
	return c
}

// Create compaction from given level and range; need external synchronization.
func (s *session) getCompactionRange(level int, umin, umax []byte) *compaction {
	v := s.version_NB()

	t0 := v.tables[level].getOverlaps(nil, s.icmp, umin, umax, level == 0)
	if len(t0) == 0 {
		return nil
	}

	// Avoid compacting too much in one shot in case the range is large.
	// But we cannot do this for level-0 since level-0 files can overlap
	// and we must not pick one file and drop another older file if the
	// two files overlap.
	if level > 0 {
		limit := uint64(kMaxTableSize)
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

	c := &compaction{s: s, v: v, level: level}
	c.tables[0] = t0
	c.expand()
	return c
}

// compaction represent a compaction state.
type compaction struct {
	s *session
	v *version

	level  int
	tables [2]tFiles

	gp              tFiles
	gpidx           int
	seenKey         bool
	overlappedBytes uint64
	imin, imax      iKey

	tPtrs [kNumLevels]int
}

// Expand compacted tables; need external synchronization.
func (c *compaction) expand() {
	level := c.level
	vt0, vt1 := c.v.tables[level], c.v.tables[level+1]

	t0, t1 := c.tables[0], c.tables[1]
	imin, imax := t0.getRange(c.s.icmp)
	t1 = vt1.getOverlaps(t1, c.s.icmp, imin.ukey(), imax.ukey(), false)
	// Get entire range covered by compaction.
	amin, amax := append(t0, t1...).getRange(c.s.icmp)

	// See if we can grow the number of inputs in "level" without
	// changing the number of "level+1" files we pick up.
	if len(t1) > 0 {
		exp0 := vt0.getOverlaps(nil, c.s.icmp, amin.ukey(), amax.ukey(), level == 0)
		if len(exp0) > len(t0) && t1.size()+exp0.size() < kExpCompactionMaxBytes {
			xmin, xmax := exp0.getRange(c.s.icmp)
			exp1 := vt1.getOverlaps(nil, c.s.icmp, xmin.ukey(), xmax.ukey(), false)
			if len(exp1) == len(t1) {
				c.s.logf("table@compaction expanding L%d+L%d (F·%d S·%s)+(F·%d S·%s) -> (F·%d S·%s)+(F·%d S·%s)",
					level, level+1, len(t0), shortenb(int(t0.size())), len(t1), shortenb(int(t1.size())),
					len(exp0), shortenb(int(exp0.size())), len(exp1), shortenb(int(exp1.size())))
				imin, imax = xmin, xmax
				t0, t1 = exp0, exp1
				amin, amax = append(t0, t1...).getRange(c.s.icmp)
			}
		}
	}

	// Compute the set of grandparent files that overlap this compaction
	// (parent == level+1; grandparent == level+2)
	if level+2 < kNumLevels {
		c.gp = c.v.tables[level+2].getOverlaps(c.gp, c.s.icmp, amin.ukey(), amax.ukey(), false)
	}

	c.tables[0], c.tables[1] = t0, t1
	c.imin, c.imax = imin, imax
}

// Check whether compaction is trivial.
func (c *compaction) trivial() bool {
	return len(c.tables[0]) == 1 && len(c.tables[1]) == 0 && c.gp.size() <= kMaxGrandParentOverlapBytes
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
	for ; c.gpidx < len(c.gp); c.gpidx++ {
		gp := c.gp[c.gpidx]
		if c.s.icmp.Compare(ikey, gp.imax) <= 0 {
			break
		}
		if c.seenKey {
			c.overlappedBytes += gp.size
		}
	}
	c.seenKey = true

	if c.overlappedBytes > kMaxGrandParentOverlapBytes {
		// Too much overlap for current output; start new output.
		c.overlappedBytes = 0
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
	}
	strict := c.s.o.GetStrict(opt.StrictIterator)

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
			it := iterator.NewIndexedIterator(tables.newIndexIterator(c.s.tops, c.s.icmp, nil, ro), strict, true)
			its = append(its, it)
		}
	}

	return iterator.NewMergedIterator(its, c.s.icmp, true)
}

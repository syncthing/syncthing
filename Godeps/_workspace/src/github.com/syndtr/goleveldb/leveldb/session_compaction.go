// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"sync/atomic"

	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/memdb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func (s *session) pickMemdbLevel(umin, umax []byte) int {
	v := s.version()
	defer v.release()
	return v.pickMemdbLevel(umin, umax)
}

func (s *session) flushMemdb(rec *sessionRecord, mdb *memdb.DB, level int) (level_ int, err error) {
	// Create sorted table.
	iter := mdb.NewIterator(nil)
	defer iter.Release()
	t, n, err := s.tops.createFrom(iter)
	if err != nil {
		return level, err
	}

	// Pick level and add to record.
	if level < 0 {
		level = s.pickMemdbLevel(t.imin.ukey(), t.imax.ukey())
	}
	rec.addTableFile(level, t)

	s.logf("memdb@flush created L%d@%d N·%d S·%s %q:%q", level, t.file.Num(), n, shortenb(int(t.size)), t.imin, t.imax)
	return level, nil
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
		// Special case for level-0.
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

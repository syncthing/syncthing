// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"errors"
	"sync/atomic"
	"unsafe"

	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var levelMaxSize [kNumLevels]float64

func init() {
	// Precompute max size of each level
	for level := range levelMaxSize {
		res := float64(10 * 1048576)
		for n := level; n > 1; n-- {
			res *= 10
		}
		levelMaxSize[level] = res
	}
}

type tSet struct {
	level int
	table *tFile
}

type version struct {
	s *session

	tables [kNumLevels]tFiles

	// Level that should be compacted next and its compaction score.
	// Score < 1 means compaction is not strictly needed. These fields
	// are initialized by computeCompaction()
	cLevel int
	cScore float64

	cSeek unsafe.Pointer

	ref  int
	next *version
}

func (v *version) release_NB() {
	v.ref--
	if v.ref > 0 {
		return
	}
	if v.ref < 0 {
		panic("negative version ref")
	}

	tables := make(map[uint64]bool)
	for _, tt := range v.next.tables {
		for _, t := range tt {
			num := t.file.Num()
			tables[num] = true
		}
	}

	for _, tt := range v.tables {
		for _, t := range tt {
			num := t.file.Num()
			if _, ok := tables[num]; !ok {
				v.s.tops.remove(t)
			}
		}
	}

	v.next.release_NB()
	v.next = nil
}

func (v *version) release() {
	v.s.vmu.Lock()
	v.release_NB()
	v.s.vmu.Unlock()
}

func (v *version) walkOverlapping(ikey iKey, f func(level int, t *tFile) bool, lf func(level int) bool) {
	ukey := ikey.ukey()

	// Walk tables level-by-level.
	for level, tables := range v.tables {
		if len(tables) == 0 {
			continue
		}

		if level == 0 {
			// Level-0 files may overlap each other. Find all files that
			// overlap ukey.
			for _, t := range tables {
				if t.overlaps(v.s.icmp, ukey, ukey) {
					if !f(level, t) {
						return
					}
				}
			}
		} else {
			if i := tables.searchMax(v.s.icmp, ikey); i < len(tables) {
				t := tables[i]
				if v.s.icmp.uCompare(ukey, t.imin.ukey()) >= 0 {
					if !f(level, t) {
						return
					}
				}
			}
		}

		if lf != nil && !lf(level) {
			return
		}
	}
}

func (v *version) get(ikey iKey, ro *opt.ReadOptions) (value []byte, tcomp bool, err error) {
	ukey := ikey.ukey()

	var (
		tset  *tSet
		tseek bool

		l0found bool
		l0seq   uint64
		l0vt    vType
		l0val   []byte
	)

	err = ErrNotFound

	// Since entries never hope across level, finding key/value
	// in smaller level make later levels irrelevant.
	v.walkOverlapping(ikey, func(level int, t *tFile) bool {
		if !tseek {
			if tset == nil {
				tset = &tSet{level, t}
			} else if tset.table.consumeSeek() <= 0 {
				tseek = true
				tcomp = atomic.CompareAndSwapPointer(&v.cSeek, nil, unsafe.Pointer(tset))
			}
		}

		ikey__, val_, err_ := v.s.tops.find(t, ikey, ro)
		switch err_ {
		case nil:
		case ErrNotFound:
			return true
		default:
			err = err_
			return false
		}

		ikey_ := iKey(ikey__)
		if seq, vt, ok := ikey_.parseNum(); ok {
			if v.s.icmp.uCompare(ukey, ikey_.ukey()) != 0 {
				return true
			}

			if level == 0 {
				if seq >= l0seq {
					l0found = true
					l0seq = seq
					l0vt = vt
					l0val = val_
				}
			} else {
				switch vt {
				case tVal:
					value = val_
					err = nil
				case tDel:
				default:
					panic("leveldb: invalid internal key type")
				}
				return false
			}
		} else {
			err = errors.New("leveldb: internal key corrupted")
			return false
		}

		return true
	}, func(level int) bool {
		if l0found {
			switch l0vt {
			case tVal:
				value = l0val
				err = nil
			case tDel:
			default:
				panic("leveldb: invalid internal key type")
			}
			return false
		}

		return true
	})

	return
}

func (v *version) getIterators(slice *util.Range, ro *opt.ReadOptions) (its []iterator.Iterator) {
	// Merge all level zero files together since they may overlap
	for _, t := range v.tables[0] {
		it := v.s.tops.newIterator(t, slice, ro)
		its = append(its, it)
	}

	strict := v.s.o.GetStrict(opt.StrictIterator) || ro.GetStrict(opt.StrictIterator)
	for _, tables := range v.tables[1:] {
		if len(tables) == 0 {
			continue
		}

		it := iterator.NewIndexedIterator(tables.newIndexIterator(v.s.tops, v.s.icmp, slice, ro), strict, true)
		its = append(its, it)
	}

	return
}

func (v *version) newStaging() *versionStaging {
	return &versionStaging{base: v}
}

// Spawn a new version based on this version.
func (v *version) spawn(r *sessionRecord) *version {
	staging := v.newStaging()
	staging.commit(r)
	return staging.finish()
}

func (v *version) fillRecord(r *sessionRecord) {
	for level, ts := range v.tables {
		for _, t := range ts {
			r.addTableFile(level, t)
		}
	}
}

func (v *version) tLen(level int) int {
	return len(v.tables[level])
}

func (v *version) offsetOf(ikey iKey) (n uint64, err error) {
	for level, tables := range v.tables {
		for _, t := range tables {
			if v.s.icmp.Compare(t.imax, ikey) <= 0 {
				// Entire file is before "ikey", so just add the file size
				n += t.size
			} else if v.s.icmp.Compare(t.imin, ikey) > 0 {
				// Entire file is after "ikey", so ignore
				if level > 0 {
					// Files other than level 0 are sorted by meta->min, so
					// no further files in this level will contain data for
					// "ikey".
					break
				}
			} else {
				// "ikey" falls in the range for this table. Add the
				// approximate offset of "ikey" within the table.
				var nn uint64
				nn, err = v.s.tops.offsetOf(t, ikey)
				if err != nil {
					return 0, err
				}
				n += nn
			}
		}
	}

	return
}

func (v *version) pickLevel(umin, umax []byte) (level int) {
	if !v.tables[0].overlaps(v.s.icmp, umin, umax, true) {
		var overlaps tFiles
		for ; level < kMaxMemCompactLevel; level++ {
			if v.tables[level+1].overlaps(v.s.icmp, umin, umax, false) {
				break
			}
			overlaps = v.tables[level+2].getOverlaps(overlaps, v.s.icmp, umin, umax, false)
			if overlaps.size() > kMaxGrandParentOverlapBytes {
				break
			}
		}
	}

	return
}

func (v *version) computeCompaction() {
	// Precomputed best level for next compaction
	var bestLevel int = -1
	var bestScore float64 = -1

	for level, tables := range v.tables {
		var score float64
		if level == 0 {
			// We treat level-0 specially by bounding the number of files
			// instead of number of bytes for two reasons:
			//
			// (1) With larger write-buffer sizes, it is nice not to do too
			// many level-0 compactions.
			//
			// (2) The files in level-0 are merged on every read and
			// therefore we wish to avoid too many files when the individual
			// file size is small (perhaps because of a small write-buffer
			// setting, or very high compression ratios, or lots of
			// overwrites/deletions).
			score = float64(len(tables)) / kL0_CompactionTrigger
		} else {
			score = float64(tables.size()) / levelMaxSize[level]
		}

		if score > bestScore {
			bestLevel = level
			bestScore = score
		}
	}

	v.cLevel = bestLevel
	v.cScore = bestScore
}

func (v *version) needCompaction() bool {
	return v.cScore >= 1 || atomic.LoadPointer(&v.cSeek) != nil
}

type versionStaging struct {
	base   *version
	tables [kNumLevels]struct {
		added   map[uint64]ntRecord
		deleted map[uint64]struct{}
	}
}

func (p *versionStaging) commit(r *sessionRecord) {
	// Deleted tables.
	for _, r := range r.deletedTables {
		tm := &(p.tables[r.level])

		if len(p.base.tables[r.level]) > 0 {
			if tm.deleted == nil {
				tm.deleted = make(map[uint64]struct{})
			}
			tm.deleted[r.num] = struct{}{}
		}

		if tm.added != nil {
			delete(tm.added, r.num)
		}
	}

	// New tables.
	for _, r := range r.addedTables {
		tm := &(p.tables[r.level])

		if tm.added == nil {
			tm.added = make(map[uint64]ntRecord)
		}
		tm.added[r.num] = r

		if tm.deleted != nil {
			delete(tm.deleted, r.num)
		}
	}
}

func (p *versionStaging) finish() *version {
	// Build new version.
	nv := &version{s: p.base.s}
	for level, tm := range p.tables {
		btables := p.base.tables[level]

		n := len(btables) + len(tm.added) - len(tm.deleted)
		if n < 0 {
			n = 0
		}
		nt := make(tFiles, 0, n)

		// Base tables.
		for _, t := range btables {
			if _, ok := tm.deleted[t.file.Num()]; ok {
				continue
			}
			if _, ok := tm.added[t.file.Num()]; ok {
				continue
			}
			nt = append(nt, t)
		}

		// New tables.
		for _, r := range tm.added {
			nt = append(nt, r.makeFile(p.base.s))
		}

		// Sort tables.
		if level == 0 {
			nt.sortByNum()
		} else {
			nt.sortByKey(p.base.s.icmp)
		}
		nv.tables[level] = nt
	}

	// Compute compaction score for new version.
	nv.computeCompaction()

	return nv
}

type versionReleaser struct {
	v    *version
	once bool
}

func (vr *versionReleaser) Release() {
	v := vr.v
	v.s.vmu.Lock()
	if !vr.once {
		v.release_NB()
		vr.once = true
	}
	v.s.vmu.Unlock()
}

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
	// Score < 1 means compaction is not strictly needed.  These fields
	// are initialized by ComputeCompaction()
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

	s := v.s

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
				s.tops.remove(t)
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

func (v *version) get(key iKey, ro *opt.ReadOptions) (value []byte, cstate bool, err error) {
	s := v.s

	ukey := key.ukey()

	var tset *tSet
	tseek := true

	// We can search level-by-level since entries never hop across
	// levels. Therefore we are guaranteed that if we find data
	// in an smaller level, later levels are irrelevant.
	for level, ts := range v.tables {
		if len(ts) == 0 {
			continue
		}

		if level == 0 {
			// Level-0 files may overlap each other. Find all files that
			// overlap user_key and process them in order from newest to
			var tmp tFiles
			for _, t := range ts {
				if s.icmp.uCompare(ukey, t.min.ukey()) >= 0 &&
					s.icmp.uCompare(ukey, t.max.ukey()) <= 0 {
					tmp = append(tmp, t)
				}
			}

			if len(tmp) == 0 {
				continue
			}

			tmp.sortByNum()
			ts = tmp
		} else {
			i := ts.searchMax(key, s.icmp)
			if i >= len(ts) || s.icmp.uCompare(ukey, ts[i].min.ukey()) < 0 {
				continue
			}

			ts = ts[i : i+1]
		}

		var l0found bool
		var l0seq uint64
		var l0type vType
		var l0value []byte
		for _, t := range ts {
			if tseek {
				if tset == nil {
					tset = &tSet{level, t}
				} else if tset.table.incrSeek() <= 0 {
					cstate = atomic.CompareAndSwapPointer(&v.cSeek, nil, unsafe.Pointer(tset))
					tseek = false
				}
			}

			var _rkey, rval []byte
			_rkey, rval, err = s.tops.get(t, key, ro)
			if err == ErrNotFound {
				continue
			} else if err != nil {
				return
			}

			rkey := iKey(_rkey)
			if seq, t, ok := rkey.parseNum(); ok {
				if s.icmp.uCompare(ukey, rkey.ukey()) == 0 {
					if level == 0 {
						if seq >= l0seq {
							l0found = true
							l0seq = seq
							l0type = t
							l0value = rval
						}
					} else {
						switch t {
						case tVal:
							value = rval
						case tDel:
							err = ErrNotFound
						default:
							panic("invalid type")
						}
						return
					}
				}
			} else {
				err = errors.New("leveldb: internal key corrupted")
				return
			}
		}
		if level == 0 && l0found {
			switch l0type {
			case tVal:
				value = l0value
			case tDel:
				err = ErrNotFound
			default:
				panic("invalid type")
			}
			return
		}
	}

	err = ErrNotFound
	return
}

func (v *version) getIterators(slice *util.Range, ro *opt.ReadOptions) (its []iterator.Iterator) {
	s := v.s

	// Merge all level zero files together since they may overlap
	for _, t := range v.tables[0] {
		it := s.tops.newIterator(t, slice, ro)
		its = append(its, it)
	}

	strict := s.o.GetStrict(opt.StrictIterator) || ro.GetStrict(opt.StrictIterator)
	for _, tt := range v.tables[1:] {
		if len(tt) == 0 {
			continue
		}

		it := iterator.NewIndexedIterator(tt.newIndexIterator(s.tops, s.icmp, slice, ro), strict, true)
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

func (v *version) offsetOf(key iKey) (n uint64, err error) {
	for level, tt := range v.tables {
		for _, t := range tt {
			if v.s.icmp.Compare(t.max, key) <= 0 {
				// Entire file is before "key", so just add the file size
				n += t.size
			} else if v.s.icmp.Compare(t.min, key) > 0 {
				// Entire file is after "key", so ignore
				if level > 0 {
					// Files other than level 0 are sorted by meta->min, so
					// no further files in this level will contain data for
					// "key".
					break
				}
			} else {
				// "key" falls in the range for this table.  Add the
				// approximate offset of "key" within the table.
				var nn uint64
				nn, err = v.s.tops.offsetOf(t, key)
				if err != nil {
					return 0, err
				}
				n += nn
			}
		}
	}

	return
}

func (v *version) pickLevel(min, max []byte) (level int) {
	if !v.tables[0].isOverlaps(min, max, false, v.s.icmp) {
		var r tFiles
		for ; level < kMaxMemCompactLevel; level++ {
			if v.tables[level+1].isOverlaps(min, max, true, v.s.icmp) {
				break
			}
			v.tables[level+2].getOverlaps(min, max, &r, true, v.s.icmp.ucmp)
			if r.size() > kMaxGrandParentOverlapBytes {
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

	for level, ff := range v.tables {
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
			score = float64(len(ff)) / kL0_CompactionTrigger
		} else {
			score = float64(ff.size()) / levelMaxSize[level]
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
	btt := p.base.tables

	// deleted tables
	for _, tr := range r.deletedTables {
		tm := &(p.tables[tr.level])

		bt := btt[tr.level]
		if len(bt) > 0 {
			if tm.deleted == nil {
				tm.deleted = make(map[uint64]struct{})
			}
			tm.deleted[tr.num] = struct{}{}
		}

		if tm.added != nil {
			delete(tm.added, tr.num)
		}
	}

	// new tables
	for _, tr := range r.addedTables {
		tm := &(p.tables[tr.level])

		if tm.added == nil {
			tm.added = make(map[uint64]ntRecord)
		}
		tm.added[tr.num] = tr

		if tm.deleted != nil {
			delete(tm.deleted, tr.num)
		}
	}
}

func (p *versionStaging) finish() *version {
	s := p.base.s
	btt := p.base.tables

	// build new version
	nv := &version{s: s}
	for level, tm := range p.tables {
		bt := btt[level]

		n := len(bt) + len(tm.added) - len(tm.deleted)
		if n < 0 {
			n = 0
		}
		nt := make(tFiles, 0, n)

		// base tables
		for _, t := range bt {
			if _, ok := tm.deleted[t.file.Num()]; ok {
				continue
			}
			if _, ok := tm.added[t.file.Num()]; ok {
				continue
			}
			nt = append(nt, t)
		}

		// new tables
		for _, tr := range tm.added {
			nt = append(nt, tr.makeFile(s))
		}

		// sort tables
		nt.sortByKey(s.icmp)
		nv.tables[level] = nt
	}

	// compute compaction score for new version
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

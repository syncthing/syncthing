// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"sync/atomic"
	"unsafe"

	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type tSet struct {
	level int
	table *tFile
}

type version struct {
	s *session

	tables []tFiles

	// Level that should be compacted next and its compaction score.
	// Score < 1 means compaction is not strictly needed. These fields
	// are initialized by computeCompaction()
	cLevel int
	cScore float64

	cSeek unsafe.Pointer

	ref int
	// Succeeding version.
	next *version
}

func newVersion(s *session) *version {
	return &version{s: s, tables: make([]tFiles, s.o.GetNumLevel())}
}

func (v *version) releaseNB() {
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

	v.next.releaseNB()
	v.next = nil
}

func (v *version) release() {
	v.s.vmu.Lock()
	v.releaseNB()
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

func (v *version) get(ikey iKey, ro *opt.ReadOptions, noValue bool) (value []byte, tcomp bool, err error) {
	ukey := ikey.ukey()

	var (
		tset  *tSet
		tseek bool

		// Level-0.
		zfound bool
		zseq   uint64
		zkt    kType
		zval   []byte
	)

	err = ErrNotFound

	// Since entries never hope across level, finding key/value
	// in smaller level make later levels irrelevant.
	v.walkOverlapping(ikey, func(level int, t *tFile) bool {
		if !tseek {
			if tset == nil {
				tset = &tSet{level, t}
			} else {
				tseek = true
			}
		}

		var (
			fikey, fval []byte
			ferr        error
		)
		if noValue {
			fikey, ferr = v.s.tops.findKey(t, ikey, ro)
		} else {
			fikey, fval, ferr = v.s.tops.find(t, ikey, ro)
		}
		switch ferr {
		case nil:
		case ErrNotFound:
			return true
		default:
			err = ferr
			return false
		}

		if fukey, fseq, fkt, fkerr := parseIkey(fikey); fkerr == nil {
			if v.s.icmp.uCompare(ukey, fukey) == 0 {
				if level == 0 {
					if fseq >= zseq {
						zfound = true
						zseq = fseq
						zkt = fkt
						zval = fval
					}
				} else {
					switch fkt {
					case ktVal:
						value = fval
						err = nil
					case ktDel:
					default:
						panic("leveldb: invalid iKey type")
					}
					return false
				}
			}
		} else {
			err = fkerr
			return false
		}

		return true
	}, func(level int) bool {
		if zfound {
			switch zkt {
			case ktVal:
				value = zval
				err = nil
			case ktDel:
			default:
				panic("leveldb: invalid iKey type")
			}
			return false
		}

		return true
	})

	if tseek && tset.table.consumeSeek() <= 0 {
		tcomp = atomic.CompareAndSwapPointer(&v.cSeek, nil, unsafe.Pointer(tset))
	}

	return
}

func (v *version) sampleSeek(ikey iKey) (tcomp bool) {
	var tset *tSet

	v.walkOverlapping(ikey, func(level int, t *tFile) bool {
		if tset == nil {
			tset = &tSet{level, t}
			return true
		} else {
			if tset.table.consumeSeek() <= 0 {
				tcomp = atomic.CompareAndSwapPointer(&v.cSeek, nil, unsafe.Pointer(tset))
			}
			return false
		}
	}, nil)

	return
}

func (v *version) getIterators(slice *util.Range, ro *opt.ReadOptions) (its []iterator.Iterator) {
	// Merge all level zero files together since they may overlap
	for _, t := range v.tables[0] {
		it := v.s.tops.newIterator(t, slice, ro)
		its = append(its, it)
	}

	strict := opt.GetStrict(v.s.o.Options, ro, opt.StrictReader)
	for _, tables := range v.tables[1:] {
		if len(tables) == 0 {
			continue
		}

		it := iterator.NewIndexedIterator(tables.newIndexIterator(v.s.tops, v.s.icmp, slice, ro), strict)
		its = append(its, it)
	}

	return
}

func (v *version) newStaging() *versionStaging {
	return &versionStaging{base: v, tables: make([]tablesScratch, v.s.o.GetNumLevel())}
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

func (v *version) pickMemdbLevel(umin, umax []byte) (level int) {
	if !v.tables[0].overlaps(v.s.icmp, umin, umax, true) {
		var overlaps tFiles
		maxLevel := v.s.o.GetMaxMemCompationLevel()
		for ; level < maxLevel; level++ {
			if v.tables[level+1].overlaps(v.s.icmp, umin, umax, false) {
				break
			}
			overlaps = v.tables[level+2].getOverlaps(overlaps, v.s.icmp, umin, umax, false)
			if overlaps.size() > uint64(v.s.o.GetCompactionGPOverlaps(level)) {
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
			score = float64(len(tables)) / float64(v.s.o.GetCompactionL0Trigger())
		} else {
			score = float64(tables.size()) / float64(v.s.o.GetCompactionTotalSize(level))
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

type tablesScratch struct {
	added   map[uint64]atRecord
	deleted map[uint64]struct{}
}

type versionStaging struct {
	base   *version
	tables []tablesScratch
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
			tm.added = make(map[uint64]atRecord)
		}
		tm.added[r.num] = r

		if tm.deleted != nil {
			delete(tm.deleted, r.num)
		}
	}
}

func (p *versionStaging) finish() *version {
	// Build new version.
	nv := newVersion(p.base.s)
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
			nt = append(nt, p.base.s.tableFileFromRecord(r))
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
		v.releaseNB()
		vr.once = true
	}
	v.s.vmu.Unlock()
}

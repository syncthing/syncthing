// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"sort"
	"sync/atomic"

	"github.com/syndtr/goleveldb/leveldb/cache"
	"github.com/syndtr/goleveldb/leveldb/comparer"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/table"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// table file
type tFile struct {
	file     storage.File
	seekLeft int32
	size     uint64
	min, max iKey
}

// test if key is after t
func (t *tFile) isAfter(key []byte, ucmp comparer.BasicComparer) bool {
	return key != nil && ucmp.Compare(key, t.max.ukey()) > 0
}

// test if key is before t
func (t *tFile) isBefore(key []byte, ucmp comparer.BasicComparer) bool {
	return key != nil && ucmp.Compare(key, t.min.ukey()) < 0
}

func (t *tFile) incrSeek() int32 {
	return atomic.AddInt32(&t.seekLeft, -1)
}

func newTFile(file storage.File, size uint64, min, max iKey) *tFile {
	f := &tFile{
		file: file,
		size: size,
		min:  min,
		max:  max,
	}

	// We arrange to automatically compact this file after
	// a certain number of seeks.  Let's assume:
	//   (1) One seek costs 10ms
	//   (2) Writing or reading 1MB costs 10ms (100MB/s)
	//   (3) A compaction of 1MB does 25MB of IO:
	//         1MB read from this level
	//         10-12MB read from next level (boundaries may be misaligned)
	//         10-12MB written to next level
	// This implies that 25 seeks cost the same as the compaction
	// of 1MB of data.  I.e., one seek costs approximately the
	// same as the compaction of 40KB of data.  We are a little
	// conservative and allow approximately one seek for every 16KB
	// of data before triggering a compaction.
	f.seekLeft = int32(size / 16384)
	if f.seekLeft < 100 {
		f.seekLeft = 100
	}

	return f
}

// table files
type tFiles []*tFile

func (tf tFiles) Len() int      { return len(tf) }
func (tf tFiles) Swap(i, j int) { tf[i], tf[j] = tf[j], tf[i] }

func (tf tFiles) lessByKey(icmp *iComparer, i, j int) bool {
	a, b := tf[i], tf[j]
	n := icmp.Compare(a.min, b.min)
	if n == 0 {
		return a.file.Num() < b.file.Num()
	}
	return n < 0
}

func (tf tFiles) lessByNum(i, j int) bool {
	return tf[i].file.Num() > tf[j].file.Num()
}

func (tf tFiles) sortByKey(icmp *iComparer) {
	sort.Sort(&tFilesSortByKey{tFiles: tf, icmp: icmp})
}

func (tf tFiles) sortByNum() {
	sort.Sort(&tFilesSortByNum{tFiles: tf})
}

func (tf tFiles) size() (sum uint64) {
	for _, t := range tf {
		sum += t.size
	}
	return sum
}

func (tf tFiles) searchMin(key iKey, icmp *iComparer) int {
	return sort.Search(len(tf), func(i int) bool {
		return icmp.Compare(tf[i].min, key) >= 0
	})
}

func (tf tFiles) searchMax(key iKey, icmp *iComparer) int {
	return sort.Search(len(tf), func(i int) bool {
		return icmp.Compare(tf[i].max, key) >= 0
	})
}

func (tf tFiles) isOverlaps(min, max []byte, disjSorted bool, icmp *iComparer) bool {
	if !disjSorted {
		// Need to check against all files
		for _, t := range tf {
			if !t.isAfter(min, icmp.ucmp) && !t.isBefore(max, icmp.ucmp) {
				return true
			}
		}
		return false
	}

	var idx int
	if len(min) > 0 {
		// Find the earliest possible internal key for min
		idx = tf.searchMax(newIKey(min, kMaxSeq, tSeek), icmp)
	}

	if idx >= len(tf) {
		// beginning of range is after all files, so no overlap
		return false
	}
	return !tf[idx].isBefore(max, icmp.ucmp)
}

func (tf tFiles) getOverlaps(min, max []byte, r *tFiles, disjSorted bool, ucmp comparer.BasicComparer) {
	for i := 0; i < len(tf); {
		t := tf[i]
		i++
		if t.isAfter(min, ucmp) || t.isBefore(max, ucmp) {
			continue
		}

		*r = append(*r, t)
		if !disjSorted {
			// Level-0 files may overlap each other.  So check if the newly
			// added file has expanded the range.  If so, restart search.
			if min != nil && ucmp.Compare(t.min.ukey(), min) < 0 {
				min = t.min.ukey()
				*r = nil
				i = 0
			} else if max != nil && ucmp.Compare(t.max.ukey(), max) > 0 {
				max = t.max.ukey()
				*r = nil
				i = 0
			}
		}
	}

	return
}

func (tf tFiles) getRange(icmp *iComparer) (min, max iKey) {
	for i, t := range tf {
		if i == 0 {
			min, max = t.min, t.max
			continue
		}
		if icmp.Compare(t.min, min) < 0 {
			min = t.min
		}
		if icmp.Compare(t.max, max) > 0 {
			max = t.max
		}
	}

	return
}

func (tf tFiles) newIndexIterator(tops *tOps, icmp *iComparer, slice *util.Range, ro *opt.ReadOptions) iterator.IteratorIndexer {
	if slice != nil {
		var start, limit int
		if slice.Start != nil {
			start = tf.searchMax(iKey(slice.Start), icmp)
		}
		if slice.Limit != nil {
			limit = tf.searchMin(iKey(slice.Limit), icmp)
		} else {
			limit = tf.Len()
		}
		tf = tf[start:limit]
	}
	return iterator.NewArrayIndexer(&tFilesArrayIndexer{
		tFiles: tf,
		tops:   tops,
		icmp:   icmp,
		slice:  slice,
		ro:     ro,
	})
}

type tFilesArrayIndexer struct {
	tFiles
	tops  *tOps
	icmp  *iComparer
	slice *util.Range
	ro    *opt.ReadOptions
}

func (a *tFilesArrayIndexer) Search(key []byte) int {
	return a.searchMax(iKey(key), a.icmp)
}

func (a *tFilesArrayIndexer) Get(i int) iterator.Iterator {
	if i == 0 || i == a.Len()-1 {
		return a.tops.newIterator(a.tFiles[i], a.slice, a.ro)
	}
	return a.tops.newIterator(a.tFiles[i], nil, a.ro)
}

type tFilesSortByKey struct {
	tFiles
	icmp *iComparer
}

func (x *tFilesSortByKey) Less(i, j int) bool {
	return x.lessByKey(x.icmp, i, j)
}

type tFilesSortByNum struct {
	tFiles
}

func (x *tFilesSortByNum) Less(i, j int) bool {
	return x.lessByNum(i, j)
}

// table operations
type tOps struct {
	s       *session
	cache   cache.Cache
	cacheNS cache.Namespace
}

func newTableOps(s *session, cacheCap int) *tOps {
	c := cache.NewLRUCache(cacheCap)
	ns := c.GetNamespace(0)
	return &tOps{s, c, ns}
}

func (t *tOps) create() (*tWriter, error) {
	file := t.s.getTableFile(t.s.allocFileNum())
	fw, err := file.Create()
	if err != nil {
		return nil, err
	}
	return &tWriter{
		t:    t,
		file: file,
		w:    fw,
		tw:   table.NewWriter(fw, t.s.o),
	}, nil
}

func (t *tOps) createFrom(src iterator.Iterator) (f *tFile, n int, err error) {
	w, err := t.create()
	if err != nil {
		return f, n, err
	}

	defer func() {
		if err != nil {
			w.drop()
		}
	}()

	for src.Next() {
		err = w.add(src.Key(), src.Value())
		if err != nil {
			return
		}
	}
	err = src.Error()
	if err != nil {
		return
	}

	n = w.tw.EntriesLen()
	f, err = w.finish()
	return
}

func (t *tOps) lookup(f *tFile) (c cache.Object, err error) {
	num := f.file.Num()
	c, ok := t.cacheNS.Get(num, func() (ok bool, value interface{}, charge int, fin cache.SetFin) {
		var r storage.Reader
		r, err = f.file.Open()
		if err != nil {
			return
		}

		o := t.s.o

		var cacheNS cache.Namespace
		if bc := o.GetBlockCache(); bc != nil {
			cacheNS = bc.GetNamespace(num)
		}

		ok = true
		value = table.NewReader(r, int64(f.size), cacheNS, o)
		charge = 1
		fin = func() {
			r.Close()
		}
		return
	})
	if !ok && err == nil {
		err = ErrClosed
	}
	return
}

func (t *tOps) get(f *tFile, key []byte, ro *opt.ReadOptions) (rkey, rvalue []byte, err error) {
	c, err := t.lookup(f)
	if err != nil {
		return nil, nil, err
	}
	defer c.Release()
	return c.Value().(*table.Reader).Find(key, ro)
}

func (t *tOps) offsetOf(f *tFile, key []byte) (offset uint64, err error) {
	c, err := t.lookup(f)
	if err != nil {
		return
	}
	_offset, err := c.Value().(*table.Reader).OffsetOf(key)
	offset = uint64(_offset)
	c.Release()
	return
}

func (t *tOps) newIterator(f *tFile, slice *util.Range, ro *opt.ReadOptions) iterator.Iterator {
	c, err := t.lookup(f)
	if err != nil {
		return iterator.NewEmptyIterator(err)
	}
	iter := c.Value().(*table.Reader).NewIterator(slice, ro)
	iter.SetReleaser(c)
	return iter
}

func (t *tOps) remove(f *tFile) {
	num := f.file.Num()
	t.cacheNS.Delete(num, func(exist bool) {
		if err := f.file.Remove(); err != nil {
			t.s.logf("table@remove removing @%d %q", num, err)
		} else {
			t.s.logf("table@remove removed @%d", num)
		}
		if bc := t.s.o.GetBlockCache(); bc != nil {
			bc.GetNamespace(num).Zap(false)
		}
	})
}

func (t *tOps) close() {
	t.cache.Zap(true)
}

type tWriter struct {
	t *tOps

	file storage.File
	w    storage.Writer
	tw   *table.Writer

	first, last []byte
}

func (w *tWriter) add(key, value []byte) error {
	if w.first == nil {
		w.first = append([]byte{}, key...)
	}
	w.last = append(w.last[:0], key...)
	return w.tw.Append(key, value)
}

func (w *tWriter) empty() bool {
	return w.first == nil
}

func (w *tWriter) finish() (f *tFile, err error) {
	err = w.tw.Close()
	if err != nil {
		return
	}
	err = w.w.Sync()
	if err != nil {
		w.w.Close()
		return
	}
	w.w.Close()
	f = newTFile(w.file, uint64(w.tw.BytesLen()), iKey(w.first), iKey(w.last))
	return
}

func (w *tWriter) drop() {
	w.w.Close()
	w.file.Remove()
	w.t.s.reuseFileNum(w.file.Num())
	w.w = nil
	w.file = nil
	w.tw = nil
	w.first = nil
	w.last = nil
}

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
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/table"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// tFile holds basic information about a table.
type tFile struct {
	file       storage.File
	seekLeft   int32
	size       uint64
	imin, imax iKey
}

// Returns true if given key is after largest key of this table.
func (t *tFile) after(icmp *iComparer, ukey []byte) bool {
	return ukey != nil && icmp.uCompare(ukey, t.imax.ukey()) > 0
}

// Returns true if given key is before smallest key of this table.
func (t *tFile) before(icmp *iComparer, ukey []byte) bool {
	return ukey != nil && icmp.uCompare(ukey, t.imin.ukey()) < 0
}

// Returns true if given key range overlaps with this table key range.
func (t *tFile) overlaps(icmp *iComparer, umin, umax []byte) bool {
	return !t.after(icmp, umin) && !t.before(icmp, umax)
}

// Cosumes one seek and return current seeks left.
func (t *tFile) consumeSeek() int32 {
	return atomic.AddInt32(&t.seekLeft, -1)
}

// Creates new tFile.
func newTableFile(file storage.File, size uint64, imin, imax iKey) *tFile {
	f := &tFile{
		file: file,
		size: size,
		imin: imin,
		imax: imax,
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

// tFiles hold multiple tFile.
type tFiles []*tFile

func (tf tFiles) Len() int      { return len(tf) }
func (tf tFiles) Swap(i, j int) { tf[i], tf[j] = tf[j], tf[i] }

// Returns true if i smallest key is less than j.
// This used for sort by key in ascending order.
func (tf tFiles) lessByKey(icmp *iComparer, i, j int) bool {
	a, b := tf[i], tf[j]
	n := icmp.Compare(a.imin, b.imin)
	if n == 0 {
		return a.file.Num() < b.file.Num()
	}
	return n < 0
}

// Returns true if i file number is greater than j.
// This used for sort by file number in descending order.
func (tf tFiles) lessByNum(i, j int) bool {
	return tf[i].file.Num() > tf[j].file.Num()
}

// Sorts tables by key in ascending order.
func (tf tFiles) sortByKey(icmp *iComparer) {
	sort.Sort(&tFilesSortByKey{tFiles: tf, icmp: icmp})
}

// Sorts tables by file number in descending order.
func (tf tFiles) sortByNum() {
	sort.Sort(&tFilesSortByNum{tFiles: tf})
}

// Returns sum of all tables size.
func (tf tFiles) size() (sum uint64) {
	for _, t := range tf {
		sum += t.size
	}
	return sum
}

// Searches smallest index of tables whose its smallest
// key is after or equal with given key.
func (tf tFiles) searchMin(icmp *iComparer, ikey iKey) int {
	return sort.Search(len(tf), func(i int) bool {
		return icmp.Compare(tf[i].imin, ikey) >= 0
	})
}

// Searches smallest index of tables whose its largest
// key is after or equal with given key.
func (tf tFiles) searchMax(icmp *iComparer, ikey iKey) int {
	return sort.Search(len(tf), func(i int) bool {
		return icmp.Compare(tf[i].imax, ikey) >= 0
	})
}

// Returns true if given key range overlaps with one or more
// tables key range. If unsorted is true then binary search will not be used.
func (tf tFiles) overlaps(icmp *iComparer, umin, umax []byte, unsorted bool) bool {
	if unsorted {
		// Check against all files.
		for _, t := range tf {
			if t.overlaps(icmp, umin, umax) {
				return true
			}
		}
		return false
	}

	i := 0
	if len(umin) > 0 {
		// Find the earliest possible internal key for min.
		i = tf.searchMax(icmp, newIKey(umin, kMaxSeq, tSeek))
	}
	if i >= len(tf) {
		// Beginning of range is after all files, so no overlap.
		return false
	}
	return !tf[i].before(icmp, umax)
}

// Returns tables whose its key range overlaps with given key range.
// If overlapped is true then the search will be expanded to tables that
// overlaps with each other.
func (tf tFiles) getOverlaps(dst tFiles, icmp *iComparer, umin, umax []byte, overlapped bool) tFiles {
	x := len(dst)
	for i := 0; i < len(tf); {
		t := tf[i]
		if t.overlaps(icmp, umin, umax) {
			if overlapped {
				// For overlapped files, check if the newly added file has
				// expanded the range. If so, restart search.
				if umin != nil && icmp.uCompare(t.imin.ukey(), umin) < 0 {
					umin = t.imin.ukey()
					dst = dst[:x]
					i = 0
					continue
				} else if umax != nil && icmp.uCompare(t.imax.ukey(), umax) > 0 {
					umax = t.imax.ukey()
					dst = dst[:x]
					i = 0
					continue
				}
			}

			dst = append(dst, t)
		}
		i++
	}

	return dst
}

// Returns tables key range.
func (tf tFiles) getRange(icmp *iComparer) (imin, imax iKey) {
	for i, t := range tf {
		if i == 0 {
			imin, imax = t.imin, t.imax
			continue
		}
		if icmp.Compare(t.imin, imin) < 0 {
			imin = t.imin
		}
		if icmp.Compare(t.imax, imax) > 0 {
			imax = t.imax
		}
	}

	return
}

// Creates iterator index from tables.
func (tf tFiles) newIndexIterator(tops *tOps, icmp *iComparer, slice *util.Range, ro *opt.ReadOptions) iterator.IteratorIndexer {
	if slice != nil {
		var start, limit int
		if slice.Start != nil {
			start = tf.searchMax(icmp, iKey(slice.Start))
		}
		if slice.Limit != nil {
			limit = tf.searchMin(icmp, iKey(slice.Limit))
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

// Tables iterator index.
type tFilesArrayIndexer struct {
	tFiles
	tops  *tOps
	icmp  *iComparer
	slice *util.Range
	ro    *opt.ReadOptions
}

func (a *tFilesArrayIndexer) Search(key []byte) int {
	return a.searchMax(a.icmp, iKey(key))
}

func (a *tFilesArrayIndexer) Get(i int) iterator.Iterator {
	if i == 0 || i == a.Len()-1 {
		return a.tops.newIterator(a.tFiles[i], a.slice, a.ro)
	}
	return a.tops.newIterator(a.tFiles[i], nil, a.ro)
}

// Helper type for sortByKey.
type tFilesSortByKey struct {
	tFiles
	icmp *iComparer
}

func (x *tFilesSortByKey) Less(i, j int) bool {
	return x.lessByKey(x.icmp, i, j)
}

// Helper type for sortByNum.
type tFilesSortByNum struct {
	tFiles
}

func (x *tFilesSortByNum) Less(i, j int) bool {
	return x.lessByNum(i, j)
}

// Table operations.
type tOps struct {
	s       *session
	cache   cache.Cache
	cacheNS cache.Namespace
}

// Creates an empty table and returns table writer.
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

// Builds table from src iterator.
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
		err = w.append(src.Key(), src.Value())
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

// Opens table. It returns a cache object, which should
// be released after use.
func (t *tOps) open(f *tFile) (c cache.Object, err error) {
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

// Finds key/value pair whose key is greater than or equal to the
// given key.
func (t *tOps) find(f *tFile, key []byte, ro *opt.ReadOptions) (rkey, rvalue []byte, err error) {
	c, err := t.open(f)
	if err != nil {
		return nil, nil, err
	}
	defer c.Release()
	return c.Value().(*table.Reader).Find(key, ro)
}

// Returns approximate offset of the given key.
func (t *tOps) offsetOf(f *tFile, key []byte) (offset uint64, err error) {
	c, err := t.open(f)
	if err != nil {
		return
	}
	_offset, err := c.Value().(*table.Reader).OffsetOf(key)
	offset = uint64(_offset)
	c.Release()
	return
}

// Creates an iterator from the given table.
func (t *tOps) newIterator(f *tFile, slice *util.Range, ro *opt.ReadOptions) iterator.Iterator {
	c, err := t.open(f)
	if err != nil {
		return iterator.NewEmptyIterator(err)
	}
	iter := c.Value().(*table.Reader).NewIterator(slice, ro)
	iter.SetReleaser(c)
	return iter
}

// Removes table from persistent storage. It waits until
// no one use the the table.
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

// Closes the table ops instance. It will close all tables,
// regadless still used or not.
func (t *tOps) close() {
	t.cache.Zap(true)
}

// Creates new initialized table ops instance.
func newTableOps(s *session, cacheCap int) *tOps {
	c := cache.NewLRUCache(cacheCap)
	ns := c.GetNamespace(0)
	return &tOps{s, c, ns}
}

// tWriter wraps the table writer. It keep track of file descriptor
// and added key range.
type tWriter struct {
	t *tOps

	file storage.File
	w    storage.Writer
	tw   *table.Writer

	first, last []byte
}

// Append key/value pair to the table.
func (w *tWriter) append(key, value []byte) error {
	if w.first == nil {
		w.first = append([]byte{}, key...)
	}
	w.last = append(w.last[:0], key...)
	return w.tw.Append(key, value)
}

// Returns true if the table is empty.
func (w *tWriter) empty() bool {
	return w.first == nil
}

// Finalizes the table and returns table file.
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
	f = newTableFile(w.file, uint64(w.tw.BytesLen()), iKey(w.first), iKey(w.last))
	return
}

// Drops the table.
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

// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"encoding/binary"
	"io/ioutil"
	"os"
	"sort"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

const (
	maxBytesInMemory = 512 << 10
)

// The IndexSorter sorts FileInfos based on their Sequence. You use it
// by first Append()ing all entries to be sorted, then calling Sorted()
// which will iterate over all the items in correctly sorted order.
type IndexSorter interface {
	Append(f protocol.FileInfo)
	Sorted(fn func(f protocol.FileInfo) bool)
	Close()
}

type internalIndexSorter interface {
	IndexSorter
	full() bool
	copyTo(to IndexSorter)
}

// NewIndexSorter returns a new IndexSorter that will start out in memory
// for efficiency but switch to on disk storage once the amount of data
// becomes large.
func NewIndexSorter(location string) IndexSorter {
	return &autoSwitchingIndexSorter{
		internalIndexSorter: newInMemoryIndexSorter(),
		location:            location,
	}
}

// An autoSwitchingSorter starts out as an inMemorySorter but becomes an
// onDiskSorter when the in memory sorter is full().
type autoSwitchingIndexSorter struct {
	internalIndexSorter
	location string
}

func (s *autoSwitchingIndexSorter) Append(f protocol.FileInfo) {
	if s.internalIndexSorter.full() {
		// We spill before adding a file instead of after, to handle the
		// case where we're over max size but won't add any more files, in
		// which case we *don't* need to spill. An example of this would be
		// an index containing just a single large file.
		l.Debugf("sorter %p spills to disk", s)
		next := newOnDiskIndexSorter(s.location)
		s.internalIndexSorter.copyTo(next)
		s.internalIndexSorter = next
	}
	s.internalIndexSorter.Append(f)
}

// An inMemoryIndexSorter is simply a slice of FileInfos. The full() method
// returns true when the number of files exceeds maxFiles or the total
// number of blocks exceeds maxBlocks.
type inMemoryIndexSorter struct {
	files    []protocol.FileInfo
	bytes    int
	maxBytes int
}

func newInMemoryIndexSorter() *inMemoryIndexSorter {
	return &inMemoryIndexSorter{
		maxBytes: maxBytesInMemory,
	}
}

func (s *inMemoryIndexSorter) Append(f protocol.FileInfo) {
	s.files = append(s.files, f)
	s.bytes += f.ProtoSize()
}

func (s *inMemoryIndexSorter) Sorted(fn func(protocol.FileInfo) bool) {
	sort.Sort(bySequence(s.files))
	for _, f := range s.files {
		if !fn(f) {
			break
		}
	}
}

func (s *inMemoryIndexSorter) Close() {
}

func (s *inMemoryIndexSorter) full() bool {
	return s.bytes >= s.maxBytes
}

func (s *inMemoryIndexSorter) copyTo(dst IndexSorter) {
	for _, f := range s.files {
		dst.Append(f)
	}
}

// bySequence sorts FileInfos by Sequence
type bySequence []protocol.FileInfo

func (l bySequence) Len() int {
	return len(l)
}
func (l bySequence) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}
func (l bySequence) Less(a, b int) bool {
	return l[a].Sequence < l[b].Sequence
}

// An onDiskIndexSorter is backed by a LevelDB database in the temporary
// directory. It relies on the fact that iterating over the database is done
// in key order and uses the Sequence as key. When done with an
// onDiskIndexSorter you must call Close() to remove the temporary database.
type onDiskIndexSorter struct {
	db  *leveldb.DB
	dir string
}

func newOnDiskIndexSorter(location string) *onDiskIndexSorter {
	// Set options to minimize resource usage.
	opts := &opt.Options{
		OpenFilesCacheCapacity: 10,
		WriteBuffer:            512 << 10,
	}

	// Use a temporary database directory.
	tmp, err := ioutil.TempDir(location, "tmp-index-sorter.")
	if err != nil {
		panic("creating temporary directory: " + err.Error())
	}
	db, err := leveldb.OpenFile(tmp, opts)
	if err != nil {
		panic("creating temporary database: " + err.Error())
	}

	s := &onDiskIndexSorter{
		db:  db,
		dir: tmp,
	}
	l.Debugf("onDiskIndexSorter %p created at %s", s, tmp)
	return s
}

func (s *onDiskIndexSorter) Append(f protocol.FileInfo) {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key[:], uint64(f.Sequence))
	data, err := f.Marshal()
	if err != nil {
		panic("bug: marshalling FileInfo should never fail: " + err.Error())
	}
	err = s.db.Put(key, data, nil)
	if err != nil {
		panic("writing to temporary database: " + err.Error())
	}
}

func (s *onDiskIndexSorter) Sorted(fn func(protocol.FileInfo) bool) {
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		var f protocol.FileInfo
		if err := f.Unmarshal(it.Value()); err != nil {
			panic("unmarshal failed: " + err.Error())
		}
		if !fn(f) {
			break
		}
	}
}

func (s *onDiskIndexSorter) Close() {
	l.Debugf("onDiskIndexSorter %p closes", s)
	s.db.Close()
	os.RemoveAll(s.dir)
}

func (s *onDiskIndexSorter) full() bool {
	return false
}

func (s *onDiskIndexSorter) copyTo(dst IndexSorter) {
	// Just wrap Sorted() if we need to support this in the future.
	panic("unsupported")
}

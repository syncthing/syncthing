// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package diskoverflow

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/syndtr/goleveldb/leveldb/iterator"
)

const suffixLength = 8

// SortValue must be implemented by every supported type for sorting. The sorting
// will happen according to bytes.Compare on the key.
type SortValue interface {
	Value
	UnmarshalWithKey(key, value []byte) SortValue // The returned SortValue must not be a reference to the receiver.
	Key() []byte
}

type Sorted struct {
	commonSorted
	key       int
	location  string
	spilling  bool
	v         SortValue
	iterating bool
}

type commonSorted interface {
	common
	add(v SortValue)
	size() int64 // Total estimated size of contents
	getFirst() (SortValue, bool)
	getLast() (SortValue, bool)
	dropFirst(v SortValue) bool
	dropLast(v SortValue) bool
	newIterator(p iteratorParent, reverse bool) SortValueIterator
}

func NewSorted(location string, v SortValue) *Sorted {
	s := &Sorted{
		key:      lim.register(),
		location: location,
		v:        v,
	}
	s.commonSorted = &memorySorted{key: s.key}
	return s
}

func (s *Sorted) Add(v SortValue) {
	if !s.spilling && !lim.add(s.key, v.Size()) {
		newSorted := newDiskSorted(s.location, s.v)
		it := s.NewIterator(false)
		for it.Next() {
			newSorted.add(it.Value())
		}
		it.Release()
		s.commonSorted.close()
		s.commonSorted = newSorted
		lim.deregister(s.key)
		s.spilling = true
	}
	s.add(v)
}

func (s *Sorted) Size() int64 {
	return s.size()
}

func (s *Sorted) Close() {
	s.close()
	if !s.spilling {
		lim.deregister(s.key)
	}
}

func (s *Sorted) NewIterator(reverse bool) SortValueIterator {
	if s.iterating {
		panic(concurrencyMsg)
	}
	s.iterating = true
	return s.newIterator(s, reverse)
}

func (s *Sorted) Length() int {
	return s.length()
}

func (s *Sorted) PopFirst() (SortValue, bool) {
	v, ok := s.getFirst()
	if ok {
		s.dropFirst(v)
	}
	return v, ok
}

func (s *Sorted) PopLast() (SortValue, bool) {
	v, ok := s.getLast()
	if ok {
		s.dropLast(v)
	}
	return v, ok
}

func (s *Sorted) String() string {
	return fmt.Sprintf("Sorted/%d", s.key)
}

func (s *Sorted) value() interface{} {
	return s.v
}

func (s *Sorted) released() {
	s.iterating = false
}

// memorySorted is basically a slice that keeps track of its size and supports
// sorted iteration of its element.
type memorySorted struct {
	droppedBytes int64
	key          int
	outgoing     bool
	values       sortSlice
}

func (s *memorySorted) add(v SortValue) {
	if s.outgoing {
		panic("Add/Append may never be called after PopFirst/PopLast")
	}
	s.values = append(s.values, v)
}

type memSortValueIterator struct {
	*memIterator
	values []SortValue
}

func (si *memSortValueIterator) Value() SortValue {
	if si.pos == si.len || si.pos == -1 {
		return nil
	}
	return si.values[si.pos]
}

func (s *memorySorted) newIterator(parent iteratorParent, reverse bool) SortValueIterator {
	if !s.outgoing {
		sort.Sort(s.values)
	}

	return &memSortValueIterator{
		memIterator: newMemIterator(parent, reverse, len(s.values)),
		values:      s.values,
	}
}

func (s *memorySorted) size() int64 {
	return lim.size(s.key) - s.droppedBytes
}

func (s *memorySorted) close() {
}

func (s *memorySorted) length() int {
	return len(s.values)
}

func (s *memorySorted) getFirst() (SortValue, bool) {
	if !s.outgoing {
		sort.Sort(s.values)
		s.outgoing = true
	}

	if s.length() == 0 {
		return nil, false
	}
	return s.values[0], true
}

func (s *memorySorted) getLast() (SortValue, bool) {
	if !s.outgoing {
		sort.Sort(s.values)
		s.outgoing = true
	}

	if s.length() == 0 {
		return nil, false
	}
	return s.values[s.length()-1], true
}

func (s *memorySorted) dropFirst(v SortValue) bool {
	return s.drop(v, s.values[1:])
}

func (s *memorySorted) dropLast(v SortValue) bool {
	return s.drop(v, s.values[:len(s.values)-1])
}

func (s *memorySorted) drop(v SortValue, newValues sortSlice) bool {
	if len(s.values) == 0 {
		return false
	}
	s.droppedBytes += v.Size()
	if s.droppedBytes > minCompactionSize && s.droppedBytes/lim.size(s.key) > 0 {
		s.values = append([]SortValue{}, newValues...)
		lim.remove(s.key, s.droppedBytes)
		s.droppedBytes = 0
	} else {
		s.values = newValues
	}
	return true
}

// diskSorted is backed by a LevelDB database in a temporary directory. It relies
// on the fact that iterating over the database is done in key order.
type diskSorted struct {
	*diskMap
	bytes int64
	v     SortValue
}

func newDiskSorted(loc string, v SortValue) *diskSorted {
	return &diskSorted{
		diskMap: newDiskMap(loc, v),
		v:       v,
	}
}

func (d *diskSorted) add(v SortValue) {
	suffix := make([]byte, suffixLength)
	binary.BigEndian.PutUint64(suffix[:], uint64(d.len))
	d.diskMap.addBytes(append(v.Key(), suffix...), v)
	d.bytes += v.Size()
}

func (d *diskSorted) size() int64 {
	return d.bytes
}

type diskIterator struct {
	it     iterator.Iterator
	v      SortValue
	parent iteratorParent
}

func (di *diskIterator) Value() SortValue {
	key := di.it.Key()
	if key == nil {
		return nil
	}
	return di.v.UnmarshalWithKey(key[:len(key)-suffixLength], di.it.Value())
}

func (di *diskIterator) Release() {
	di.it.Release()
	di.parent.released()
}

type diskForwardIterator struct {
	*diskIterator
}

func (i *diskForwardIterator) Next() bool {
	out := i.it.Next()
	return out
	// return i.it.Next()
}

type diskReverseIterator struct {
	*diskIterator
	next func(*diskReverseIterator) bool
}

func (i *diskReverseIterator) Next() bool {
	return i.next(i)
}

func (d *diskSorted) newIterator(parent iteratorParent, reverse bool) SortValueIterator {
	di := &diskIterator{
		it:     d.db.NewIterator(nil, nil),
		v:      d.v,
		parent: parent,
	}
	if reverse {
		ri := &diskReverseIterator{diskIterator: di}
		ri.next = func(i *diskReverseIterator) bool {
			i.next = func(j *diskReverseIterator) bool {
				return j.it.Prev()
			}
			return i.it.Last()
		}
		return ri
	}
	return &diskForwardIterator{di}
}

func (d *diskSorted) getFirst() (SortValue, bool) {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()
	if !it.First() {
		return nil, false
	}
	key := it.Key()
	return d.v.UnmarshalWithKey(key[:len(key)-suffixLength], it.Value()), true
}

func (d *diskSorted) getLast() (SortValue, bool) {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()
	if !it.Last() {
		return nil, false
	}
	key := it.Key()
	return d.v.UnmarshalWithKey(key[:len(key)-suffixLength], it.Value()), true
}

func (d *diskSorted) dropFirst(v SortValue) bool {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()
	if !it.First() {
		return false
	}
	d.db.Delete(it.Key(), nil)
	d.bytes -= v.Size()
	d.len--
	return true
}

func (d *diskSorted) dropLast(v SortValue) bool {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()
	if !it.Last() {
		return false
	}
	d.db.Delete(it.Key(), nil)
	d.bytes -= v.Size()
	d.len--
	return true
}

// sortSlice is a sortable slice of sortValues
type sortSlice []SortValue

func (s sortSlice) Len() int {
	return len(s)
}
func (s sortSlice) Swap(a, b int) {
	s[a], s[b] = s[b], s[a]
}
func (s sortSlice) Less(a, b int) bool {
	return bytes.Compare(s[a].Key(), s[b].Key()) == -1
}

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
	key      int
	location string
	spilling bool
	v        SortValue
}

type commonSorted interface {
	common
	add(v SortValue)
	size() int64 // Total estimated size of contents
	iter(fn func(SortValue) bool, rev, closing bool) bool
	getFirst() (SortValue, bool)
	getLast() (SortValue, bool)
	dropFirst(v SortValue) bool
	dropLast(v SortValue) bool
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
		s.iter(func(v SortValue) bool {
			newSorted.add(v)
			return true
		}, false, true)
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

func (s *Sorted) Iter(fn func(SortValue) bool, rev bool) {
	s.iter(fn, rev, false)
}

func (s *Sorted) IterAndClose(fn func(SortValue) bool, rev bool) {
	s.iter(fn, rev, true)
	s.Close()
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

func (s *memorySorted) iter(fn func(SortValue) bool, rev, closing bool) bool {
	if closing {
		defer s.close()
	}

	if !s.outgoing {
		sort.Sort(s.values)
	}

	orig := lim.size(s.key)
	removed := int64(0)
	for i := range s.values {
		if rev {
			i = len(s.values) - 1 - i
		}
		if !fn(s.values[i]) {
			return false
		}
		if closing && orig > 2*minCompactionSize {
			removed += s.values[i].Size()
			if removed > minCompactionSize && removed/orig > 0 {
				s.values = append([]SortValue{}, s.values[i+1:]...)
				lim.remove(s.key, removed)
				i = 0
				removed = 0
			}
		}
	}
	return true
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
	if s.length() == 0 {
		return false
	}
	s.droppedBytes += v.Size()
	if s.droppedBytes > minCompactionSize && s.droppedBytes/lim.size(s.key) > 0 {
		s.values = append([]SortValue{}, s.values[1:]...)
		lim.remove(s.key, s.droppedBytes)
		s.droppedBytes = 0
	} else {
		s.values = s.values[1:]
	}
	return true
}

func (s *memorySorted) dropLast(v SortValue) bool {
	if len(s.values) == 0 {
		return false
	}
	s.droppedBytes += v.Size()
	if s.droppedBytes > minCompactionSize && s.droppedBytes/lim.size(s.key) > 0 {
		s.values = append([]SortValue{}, s.values[:len(s.values)-1]...)
		lim.remove(s.key, s.droppedBytes)
		s.droppedBytes = 0
	} else {
		s.values = s.values[:len(s.values)-1]
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

func (d *diskSorted) iter(fn func(SortValue) bool, rev, closing bool) bool {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()
	init := it.First
	step := it.Next
	if rev {
		init = it.Last
		step = it.Prev
	}
	for ok := init(); ok; ok = step() {
		key := it.Key()
		v := d.v.UnmarshalWithKey(key[:len(key)-suffixLength], it.Value())
		if !fn(v) {
			return false
		}
	}
	return true
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

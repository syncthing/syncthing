// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package diskoverflow

import (
	"bytes"
	"sort"
)

// Sorted stores a list of SortValue sorted by the return value of their Key method.
// After calling PopFirst or PopLast for the first time, calls to Add will panic.
type Sorted interface {
	Common
	Add(v SortValue)
	Iter(fn func(v Value) bool, rev bool)
	IterAndClose(fn func(v Value) bool, rev bool)
	PopFirst() (Value, bool)
	PopLast() (Value, bool)
}

func NewSorted(location string) Sorted {
	s := &sorted{
		key:      lim.register(),
		location: location,
	}
	s.commonSorted = &memorySorted{key: s.key}
	return s
}

// SortValue must be implemented by every supported type for sorting. The sorting
// will happen according to bytes.Compare on the key.
type SortValue interface {
	Value
	Key() []byte
}

type commonSorted interface {
	common
	add(v SortValue)
	bytes() int64 // Total estimated size of contents
	iter(fn func(v SortValue) bool, rev, closing bool)
	getFirst() (SortValue, bool)
	getLast() (SortValue, bool)
	dropFirst() bool
	dropLast() bool
}

type sorted struct {
	commonSorted
	inactive commonSorted
	key      int
	location string
	spilling bool
}

func (s *sorted) Add(v SortValue) {
	if !s.spilling && lim.add(s.key, v.Bytes()) {
		s.inactive = s.commonSorted
		s.commonSorted = &diskSorted{diskMap: newDiskMap(s.location)}
	}
	s.add(v)
}

func (s *sorted) Bytes() int64 {
	if s.spilling {
		return s.bytes() + lim.bytes(s.key)
	}
	return lim.bytes(s.key)
}

func (s *sorted) Close() {
	s.close()
}

func (s *sorted) Iter(fn func(v Value) bool, rev bool) {
	s.iterImpl(fn, rev, false)
}

func (s *sorted) IterAndClose(fn func(v Value) bool, rev bool) {
	s.iterImpl(fn, rev, true)
	s.Close()
}

func (s *sorted) iterImpl(fn func(v Value) bool, rev, closing bool) {
	if closing {
		defer s.Close()
	}
	if !s.spilling {
		s.iter(func(v SortValue) bool {
			return fn(v)
		}, rev, closing)
		return
	}
	aChan := make(chan SortValue)
	iChan := make(chan SortValue)
	go func() {
		s.iter(func(v SortValue) bool {
			aChan <- v
			return true
		}, rev, closing)
		close(aChan)
	}()
	go func() {
		s.iter(func(v SortValue) bool {
			iChan <- v
			return true
		}, rev, closing)
		close(iChan)
	}()
	av, aok := <-aChan
	iv, iok := <-iChan
	comp := -1
	if rev {
		comp = 1
	}
	for aok && iok {
		if bytes.Compare(av.Key(), iv.Key()) == comp {
			if !fn(av) {
				return
			}
			av, aok = <-aChan
			continue
		}
		if !fn(iv) {
			return
		}
		iv, iok = <-iChan
	}
	for ; aok; av, aok = <-aChan {
		if !fn(av) {
			return
		}
	}
	for ; iok; iv, iok = <-iChan {
		if !fn(iv) {
			return
		}
	}
}

func (s *sorted) Length() int {
	return s.length()
}

func (s *sorted) PopFirst() (Value, bool) {
	a, aok := s.getFirst()
	if !s.spilling {
		s.dropFirst()
		return a, aok
	}
	i, iok := s.inactive.getFirst()
	if !aok {
		s.inactive.dropFirst()
		return i, iok
	}
	if !iok || bytes.Compare(a.Key(), i.Key()) == -1 {
		s.dropFirst()
		return a, aok
	}
	s.inactive.dropFirst()
	return i, iok
}

func (s *sorted) PopLast() (Value, bool) {
	a, aok := s.getFirst()
	if !s.spilling {
		s.dropFirst()
		return a, aok
	}
	i, iok := s.inactive.getFirst()
	if !aok {
		s.inactive.dropFirst()
		return i, iok
	}
	if !iok || bytes.Compare(a.Key(), i.Key()) == 1 {
		s.dropFirst()
		return a, aok
	}
	s.inactive.dropFirst()
	return i, iok
}

// memorySorted is basically a slice that keeps track of its size and supports
// sorted iteration of its element.
type memorySorted struct {
	slice
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

func (s *memorySorted) iter(fn func(SortValue) bool, rev, closing bool) {
	if closing {
		defer s.close()
	}

	if !s.outgoing {
		sort.Sort(s.values)
	}

	orig := s.bytes()
	removed := int64(0)
	for i := range s.values {
		if rev {
			i = len(s.values) - 1 - i
		}
		if !fn(s.values[i]) {
			return
		}
		if closing {
			removed += s.values[i].Bytes()
			if removed > minCompactionSize && removed/orig > 0 {
				s.values = append([]SortValue{}, s.values[i+1:]...)
				lim.remove(s.key, removed)
				i = 0
				removed = 0
			}
		}
	}
}

func (s *memorySorted) bytes() int64 {
	return lim.bytes(s.key) - s.droppedBytes
}

func (s *memorySorted) close() {
}

func (s *memorySorted) length() int {
	return len(s.values)
}

func (s *memorySorted) getFirst() (v SortValue, ok bool) {
	if !s.outgoing {
		sort.Sort(s.values)
		s.outgoing = true
	}

	if s.length() == 0 {
		return nil, false
	}
	return s.values[0], true
}

func (s *memorySorted) getLast() (v SortValue, ok bool) {
	if !s.outgoing {
		sort.Sort(s.values)
		s.outgoing = true
	}

	if s.length() == 0 {
		return nil, false
	}
	return s.values[s.length()-1], true
}

func (s *memorySorted) dropFirst() bool {
	if s.length() == 0 {
		return false
	}
	s.droppedBytes += s.values[0].Bytes()
	if s.droppedBytes > minCompactionSize && s.droppedBytes/lim.bytes(s.key) > 0 {
		s.values = append([]SortValue{}, s.values[1:]...)
		lim.remove(s.key, s.droppedBytes)
		s.droppedBytes = 0
	} else {
		s.values = s.values[1:]
	}
	return true
}

func (s *memorySorted) dropLast() bool {
	if len(s.values) == 0 {
		return false
	}
	s.droppedBytes += s.values[len(s.values)-1].Bytes()
	if s.droppedBytes > minCompactionSize && s.droppedBytes/lim.bytes(s.key) > 0 {
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
	size int64
}

func (d *diskSorted) add(v SortValue) {
	d.diskMap.addBytes(v.Key(), v)
	d.size += v.Bytes()
}

func (d *diskSorted) bytes() int64 {
	return d.size
}

func (d *diskSorted) iter(fn func(SortValue) bool, rev, closing bool) {
	if !rev {
		d.diskMap.iter(func(_ string, v Value) bool { return fn(v.(SortValue)) })
	}
	it := d.db.NewIterator(nil, nil)
	defer it.Release()
	it.Last()
	for it.Prev() {
		var v SortValue
		v.FromByte(it.Value())
		if !fn(v) {
			return
		}
	}
}

func (d *diskSorted) getFirst() (SortValue, bool) {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()
	if !it.First() {
		return nil, false
	}
	var v SortValue
	v.FromByte(it.Value())
	return v, true
}

func (d *diskSorted) getLast() (SortValue, bool) {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()
	if !it.Last() {
		return nil, false
	}
	var v SortValue
	v.FromByte(it.Value())
	return v, true
}

func (d *diskSorted) dropFirst() bool {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()
	if !it.First() {
		return false
	}
	d.db.Delete(it.Key(), nil)
	return true
}

func (d *diskSorted) dropLast() bool {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()
	if !it.First() {
		return false
	}
	d.db.Delete(it.Key(), nil)
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

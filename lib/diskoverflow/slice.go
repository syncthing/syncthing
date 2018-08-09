// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package diskoverflow

import (
	"encoding/binary"
	"fmt"
)

type Slice struct {
	commonSlice
	inactive  commonSlice
	location  string
	key       int
	spilling  bool
	v         Value
	iterating bool
}

type commonSlice interface {
	common
	append(v Value)
	size() int64
	newIterator(p iteratorParent, reverse bool) ValueIterator
}

func NewSlice(location string, v Value) *Slice {
	s := &Slice{
		key:      lim.register(),
		location: location,
		v:        v,
	}
	s.commonSlice = &memorySlice{key: s.key}
	return s
}

func (s *Slice) Append(v Value) {
	if s.iterating {
		panic(concurrencyMsg)
	}
	if !s.spilling && !lim.add(s.key, v.Size()) {
		s.inactive = s.commonSlice
		s.commonSlice = &diskSlice{newDiskSorted(s.location, &nonSortValue{Value: s.v})}
		s.spilling = true
	}
	s.append(v)
}

func (s *Slice) Size() int64 {
	if s.spilling {
		return s.size() + s.inactive.size()
	}
	return s.size()
}

func (s *Slice) Close() {
	s.close()
	if s.spilling {
		s.inactive.close()
	}
	lim.deregister(s.key)
}

func (s *Slice) value() interface{} {
	return s.v
}

func (s *Slice) released() {
	s.iterating = false
}

type sliceIterator struct {
	ValueIterator
	inactive      ValueIterator
	firstIterator bool
}

func (s *Slice) NewIterator(reverse bool) ValueIterator {
	if s.iterating {
		panic(concurrencyMsg)
	}
	s.iterating = true
	if !s.spilling {
		return s.newIterator(s, reverse)
	}
	it := &sliceIterator{
		firstIterator: true,
	}
	if reverse {
		it.ValueIterator = s.newIterator(s, reverse)
		it.inactive = s.inactive.newIterator(s, reverse)
	} else {
		it.ValueIterator = s.inactive.newIterator(s, reverse)
		it.inactive = s.newIterator(s, reverse)
	}
	return it
}

func (si *sliceIterator) switchIterators() {
	tmp := si.inactive
	si.inactive = si.ValueIterator
	si.ValueIterator = tmp
	si.firstIterator = false
}

func (si *sliceIterator) Next() bool {
	if si.ValueIterator.Next() {
		return true
	}
	if !si.firstIterator {
		return false
	}
	si.switchIterators()
	return si.ValueIterator.Next()
}

func (si *sliceIterator) Release() {
	si.ValueIterator.Release()
	si.inactive.Release()
}

func (s *Slice) Length() int {
	if !s.spilling {
		return s.length()
	}
	return s.length() + s.inactive.length()
}

func (s *Slice) String() string {
	return fmt.Sprintf("Slice/%d", s.key)
}

type memorySlice struct {
	key          int
	values       []Value
	droppedBytes int64
}

func (s *memorySlice) append(v Value) {
	s.values = append(s.values, v)
}

func (s *memorySlice) size() int64 {
	return lim.size(s.key)
}

func (s *memorySlice) close() {
	s.values = nil
}

type memValueIterator struct {
	*memIterator
	values []Value
}

func (s *memorySlice) newIterator(p iteratorParent, reverse bool) ValueIterator {
	return &memValueIterator{
		memIterator: newMemIterator(p, reverse, len(s.values)),
		values:      s.values,
	}
}

func (si *memValueIterator) Value() Value {
	if si.pos == si.len || si.pos == -1 {
		return nil
	}
	return si.values[si.pos]
}

func (s *memorySlice) length() int {
	return len(s.values)
}

type diskSlice struct {
	*diskSorted
}

func (s *diskSlice) append(v Value) {
	s.diskSorted.add(&nonSortValue{v, uint64(s.len)})
}

type diskSliceIterator struct {
	SortValueIterator
}

func (i *diskSliceIterator) Value() Value {
	sv := i.SortValueIterator.Value()
	if sv == nil {
		return nil
	}
	return sv.(*nonSortValue).Value
}

func (s *diskSlice) newIterator(p iteratorParent, reverse bool) ValueIterator {
	return &diskSliceIterator{s.diskSorted.newIterator(p, reverse)}
}

// nonSortValue implements the SortValue interface, to be "sorted" by insertion order.
type nonSortValue struct {
	Value
	index uint64
}

func (s *nonSortValue) Key() []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key[:], uint64(s.index))
	return key
}

func (s *nonSortValue) UnmarshalWithKey(key, value []byte) SortValue {
	return &nonSortValue{
		Value: s.Value.Unmarshal(value),
		index: binary.BigEndian.Uint64(key),
	}
}

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
	inactive commonSlice
	location string
	key      int
	spilling bool
	v        Value
}

type commonSlice interface {
	common
	append(v Value)
	size() int64
	iter(fn func(v Value) bool, rev bool, closing bool) bool
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

func (s *Slice) Iter(fn func(v Value) bool, rev bool) {
	s.iterImpl(fn, rev, false)
}

func (s *Slice) IterAndClose(fn func(Value) bool, rev bool) {
	s.iterImpl(fn, rev, true)
	s.Close()
}

func (s *Slice) iterImpl(fn func(Value) bool, rev, closing bool) {
	if !s.spilling {
		s.iter(fn, rev, closing)
		return
	}
	if rev {
		if s.iter(fn, true, closing) {
			s.inactive.iter(fn, true, closing)
		}
		return
	}
	if s.inactive.iter(fn, false, closing) {
		s.iter(fn, false, closing)
	}
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
	key    int
	values []Value
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

func (s *memorySlice) iter(fn func(Value) bool, rev, closing bool) bool {
	if closing {
		defer s.close()
	}
	orig := s.size()
	removed := int64(0)
	for k := 0; k < len(s.values); k++ {
		i := k
		if rev {
			i = len(s.values) - 1 - k
		}
		if !fn(s.values[i]) {
			return false
		}
		if closing && orig > 2*minCompactionSize {
			removed += s.values[i].Size()
			if removed > minCompactionSize && removed/orig > 0 {
				s.values = append([]Value{}, s.values[i+1:]...)
				lim.remove(s.key, removed)
				i = 0
				removed = 0
			}
		}
	}
	return true
}

func (s *memorySlice) length() int {
	return len(s.values)
}

type diskSlice struct {
	*diskSorted
}

func (s *diskSlice) iter(fn func(Value) bool, rev, closing bool) bool {
	if closing {
		defer s.close()
	}
	return s.diskSorted.iter(func(sv SortValue) bool {
		return fn(sv.(*nonSortValue).Value)
	}, rev, closing)
}

func (s *diskSlice) append(v Value) {
	s.diskSorted.add(&nonSortValue{v, uint64(s.len)})
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

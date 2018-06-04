// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package diskoverflow

import (
	"encoding/binary"
)

type Slice struct {
	commonSlice
	inactive commonSlice
	location string
	key      int
	spilling bool
}

type commonSlice interface {
	common
	append(v Value)
	bytes() int64
	iter(fn func(v Value) bool, rev bool, closing bool) bool
}

func NewSlice(location string) Slice {
	s := &Slice{
		key:      lim.register(),
		location: location,
	}
	s.commonSlice = &memorySlice{key: s.key}
	return s
}

func (s *Slice) Append(v Value) {
	if !s.spilling && !lim.add(s.key, v.Size()) {
		s.inactive = s.commonSlice
		s.commonSlice = &diskSlice{&diskSorted{diskMap: newDiskMap(s.location)}}
	}
	s.append(v)
}

func (s *Slice) Bytes() int64 {
	if s.spilling {
		return s.bytes() + lim.bytes(s.key)
	}
	return lim.bytes(s.key)
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

type memorySlice struct {
	key    int
	values []Value
}

func (s *memorySlice) append(v Value) {
	s.values = append(s.values, v)
}

func (s *memorySlice) bytes() int64 {
	return lim.bytes(s.key)
}

func (s *memorySlice) close() {
	s.values = nil
}

func (s *memorySlice) iter(fn func(Value) bool, rev, closing bool) bool {
	if closing {
		defer s.close()
	}
	orig := s.bytes()
	removed := int64(0)
	for i := 0; i < len(s.values); i++ {
		if rev {
			i = len(s.values) - 1 - i
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
	return s.diskSorted.iter(func(v SortValue) bool { return fn(v.(nonSortValue).Value) }, rev, closing)
}

func (s *diskSlice) append(v Value) {
	s.diskSorted.add(&nonSortValue{v, s.len})
}

// nonSortValue implements the SortValue interface, to be "sorted" by insertion order.
type nonSortValue struct {
	Value
	index int
}

func (s nonSortValue) Key() []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key[:], uint64(s.index))
	return key
}

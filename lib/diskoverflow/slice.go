// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package diskoverflow

import (
	"encoding/binary"
)

type Slice interface {
	Common
	Append(v Value)
	Bytes() int64 // Total estimated size of contents
	Iter(fn func(v Value) bool, rev bool)
	IterAndClose(fn func(v Value) bool, rev bool)
}

func NewSlice(location string) Slice {
	s := &slice{
		key:      lim.register(),
		location: location,
	}
	s.commonSlice = &memorySlice{key: s.key}
	return s
}

type commonSlice interface {
	common
	append(v Value)
	bytes() int64
	iter(fn func(v Value) bool, rev bool, closing bool)
}

type slice struct {
	commonSlice
	inactive commonSlice
	location string
	key      int
	spilling bool
}

func (s *slice) Append(v Value) {
	if !s.spilling && lim.add(s.key, v.Bytes()) {
		s.inactive = s.commonSlice
		s.commonSlice = &diskSlice{&diskSorted{diskMap: newDiskMap(s.location)}}
	}
	s.append(v)
}

func (s *slice) Bytes() int64 {
	if s.spilling {
		return s.bytes() + lim.bytes(s.key)
	}
	return lim.bytes(s.key)
}

func (s *slice) Close() {
	s.close()
	if s.spilling {
		s.inactive.close()
	}
	lim.deregister(s.key)
}

func (s *slice) Iter(fn func(v Value) bool, rev bool) {
	s.iterImpl(fn, rev, false)
}

func (s *slice) IterAndClose(fn func(Value) bool, rev bool) {
	s.iterImpl(fn, rev, true)
	s.Close()
}

func (s *slice) iterImpl(fn func(Value) bool, rev, closing bool) {
	if !s.spilling {
		s.iter(fn, rev, closing)
		return
	}
	if rev {
		s.iter(fn, true, closing)
		s.inactive.iter(fn, true, closing)
		return
	}
	s.inactive.iter(fn, false, closing)
	s.iter(fn, false, closing)
}

func (s *slice) Length() int {
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

func (s *memorySlice) iter(fn func(Value) bool, rev, closing bool) {
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
			return
		}
		if closing {
			removed += s.values[i].Bytes()
			if removed > minCompactionSize && removed/orig > 0 {
				s.values = append([]Value{}, s.values[i+1:]...)
				lim.remove(s.key, removed)
				i = 0
				removed = 0
			}
		}
	}
}

func (s *memorySlice) length() int {
	return len(s.values)
}

type diskSlice struct {
	*diskSorted
}

func (s *diskSlice) iter(fn func(Value) bool, rev, closing bool) {
	s.diskSorted.iter(func(v SortValue) bool { return fn(v.(nonSortValue).Value) }, rev, closing)
	if closing {
		s.close()
	}
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

// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package diskoverflow provides several data container types which are limited
// in their memory usage. Once the total memory limit is reached, all new data
// is written to disk.
// Do not use any instances of these types concurrently!
package diskoverflow

import (
	"sync"

	"github.com/syncthing/syncthing/lib/protocol"
)

const OrigDefaultOverflowBytes int64 = 16 << protocol.MiB

var (
	defaultOverflow    int64 = OrigDefaultOverflowBytes
	defaultOverflowMut       = sync.Mutex{}
)

// Sets the default limit of when data is written to disks. Can be overruled
// on individual instances via their SetOverflowBytes method.
// Argument bytes < 0 means no disk spilling will ever happen and = 0 sets it to
// OrigDefaultOverflowbytes.
func SetDefaultOverflowBytes(bytes int64) {
	defaultOverflowMut.Lock()
	if bytes == 0 {
		defaultOverflow = OrigDefaultOverflowBytes
	} else {
		defaultOverflow = bytes
	}
	defaultOverflowMut.Unlock()
}

func DefaultOverflowBytes() int64 {
	defaultOverflowMut.Lock()
	defer defaultOverflowMut.Unlock()
	return defaultOverflow
}

// Value must be implemented by every type that is to be stored in a disk spilling container.
type Value interface {
	Bytes() int64
	Marshal() []byte
	Unmarshal([]byte)
}

// ValueFileInfo implements Value for protocol.FileInfo
type ValueFileInfo struct{ protocol.FileInfo }

func (s *ValueFileInfo) Bytes() int64 {
	return int64(s.ProtoSize())
}

func (s *ValueFileInfo) Marshal() []byte {
	data, err := s.FileInfo.Marshal()
	if err != nil {
		panic("bug: marshalling FileInfo should never fail: " + err.Error())
	}
	return data
}

func (s *ValueFileInfo) Unmarshal(v []byte) {
	if err := s.FileInfo.Unmarshal(v); err != nil {
		panic("unmarshal failed: " + err.Error())
	}
}

type common interface {
	Bytes() int64
	Items() int
	Close()
}

type base struct {
	location      string
	overflowBytes int64
	spilling      bool
	iterating     bool
}

func newBase(location string) base {
	defaultOverflowMut.Lock()
	defer defaultOverflowMut.Unlock()
	return base{
		location:      location,
		overflowBytes: defaultOverflow,
	}
}

// SetOverflowBytes changes the limit of when contents are written to disk.
// A change only takes effect if another element is added to the container.
// A value of <= 0 means no disk spilling will ever happen.
func (o *base) SetOverflowBytes(bytes int64) {
	o.overflowBytes = bytes
}

func (o *base) startSpilling(size int64) bool {
	return !o.spilling && o.overflowBytes > 0 && size > o.overflowBytes
}

type Iterator interface {
	Release()
	Next() bool
}

type ValueIterator interface {
	Iterator
	Value() Value
}

type SortValueIterator interface {
	Iterator
	Value() SortValue
}

type iteratorParent interface {
	released()
}

const (
	concurrencyMsg = "iteration in progress - don't modify or start a new iteration concurrently"
)

type memIterator struct {
	pos     int
	len     int
	reverse bool
	parent  iteratorParent
}

func newMemIterator(p iteratorParent, reverse bool, len int) *memIterator {
	it := &memIterator{
		len:     len,
		reverse: reverse,
		parent:  p,
	}
	if reverse {
		it.pos = len
	} else {
		it.pos = -1
	}
	return it
}

func (si *memIterator) Next() bool {
	if si.reverse {
		if si.pos == 0 {
			return false
		}
		si.pos--
		return true
	}
	if si.pos == si.len-1 {
		return false
	}
	si.pos++
	return true
}

func (si *memIterator) Release() {
	si.parent.released()
}

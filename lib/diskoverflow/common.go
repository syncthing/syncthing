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
	"reflect"
	"sync"

	"github.com/syncthing/syncthing/lib/protocol"
)

const concurrencyMsg = "iteration in progress - don't modify or start a new iteration concurrently"

const OrigDefaultOverflowBytes = 16 << protocol.MiB

var (
	defaultOverflow    = OrigDefaultOverflowBytes
	defaultOverflowMut = sync.Mutex{}
)

// Sets the default limit of when data is written to disks. Can be overruled
// on individual instances via their SetOverflowBytes method.
// A value of bytes =< 0 means no disk spilling will ever happen.
func SetDefaultOverflowBytes(bytes int) {
	defaultOverflowMut.Lock()
	defaultOverflow = bytes
	defaultOverflowMut.Unlock()
}

func DefaultOverflowBytes() int {
	defaultOverflowMut.Lock()
	defer defaultOverflowMut.Unlock()
	return defaultOverflow
}

// Value must be implemented by every type that is to be stored in a disk spilling container.
type Value interface {
	Bytes() int
	Marshal() []byte
	Unmarshal([]byte)
	Reset() // To make an already populated Value ready for Unmarshal
}

// copyValue copies the content from src to dst. Src and dst must be pointers
// to the same underlying types, otherwise this will panic.
func copyValue(dst, src Value) {
	dstv := reflect.ValueOf(dst).Elem()
	srcv := reflect.ValueOf(src).Elem()
	dstv.Set(srcv)
}

// ValueFileInfo implements Value for protocol.FileInfo
type ValueFileInfo struct{ protocol.FileInfo }

func (s *ValueFileInfo) Bytes() int {
	return s.ProtoSize()
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

func (s *ValueFileInfo) Reset() {
	s.FileInfo = protocol.FileInfo{}
}

type common interface {
	Bytes() int
	Items() int
	Close()
}

type base struct {
	location      string
	overflowBytes int
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

func (o *base) SetOverflowBytes(bytes int) {
	o.overflowBytes = bytes
}

func (o *base) startSpilling(size int) bool {
	return !o.spilling && o.overflowBytes > 0 && size > o.overflowBytes
}

type Iterator interface {
	Release()
	Next() bool
	Value(Value)
}

type iteratorParent interface {
	released()
}

type memIterator struct {
	values  []Value
	pos     int
	len     int
	reverse bool
	parent  iteratorParent
}

func newMemIterator(values []Value, p iteratorParent, reverse bool, len int) *memIterator {
	it := &memIterator{
		values:  values,
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

func (si *memIterator) Value(v Value) {
	if si.pos != si.len && si.pos != -1 {
		copyValue(v, si.values[si.pos])
	}
}

func (si *memIterator) Release() {
	si.parent.released()
}

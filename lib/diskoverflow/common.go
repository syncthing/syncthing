// Copyright (C) 2018 The Syncthing Authors.
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
// Un-/Marshal returning an error will result in a panic.
// This uses methods from proto interfaces such that existing types can be reused.
type Value interface {
	Marshal() ([]byte, error)
	Unmarshal([]byte) error
	Reset() // To make an already populated Value ready for Unmarshal
	ProtoSize() int
}

// Common are the methods implemented by all container types.
//
// Close must be called to release resources. All iterations must be released
// before calling Close.
type Common interface {
	Bytes() int
	Items() int
	SetOverflowBytes(bytes int)
	Close()
}

// copyValue copies the content from src to dst. Src and dst must be pointers
// to the same underlying types, otherwise this will panic.
func copyValue(dst, src Value) {
	dstv := reflect.ValueOf(dst).Elem()
	srcv := reflect.ValueOf(src).Elem()
	dstv.Set(srcv)
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

type posIterator struct {
	len     int
	offset  int
	reverse bool
}

func newPosIterator(l int, reverse bool) *posIterator {
	return &posIterator{
		len:     l,
		offset:  -1,
		reverse: reverse,
	}
}

func (si *posIterator) pos() int {
	if !si.reverse {
		return si.offset
	}
	return si.len - si.offset - 1
}

func (si *posIterator) Next() bool {
	if si.offset == si.len-1 {
		return false
	}
	si.offset++
	return true
}

func (si *posIterator) Release() {}

func errPanic(err error) {
	if err != nil {
		panic(err)
	}
}

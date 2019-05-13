// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package diskoverflow

import (
	"fmt"
)

type Slice struct {
	commonSlice
	base
}

type commonSlice interface {
	common
	append(v Value)
	newIterator(p iteratorParent, reverse bool) ValueIterator
}

// NewSorted creates a slice like container, spilling to disk at location.
// All items added to this instance must be of the same type as v.
func NewSlice(location string) *Slice {
	o := &Slice{base: newBase(location)}
	o.commonSlice = &memorySlice{}
	return o
}

func (o *Slice) Append(v Value) {
	if o.iterating {
		panic(concurrencyMsg)
	}
	if o.startSpilling(o.Bytes() + v.Bytes()) {
		d := v.Marshal()
		ds := &diskSlice{newDiskSorted(o.location)}
		it := o.newIterator(o, false)
		for it.Next() {
			v.Reset()
			it.Value(v)
			ds.append(v)
		}
		it.Release()
		o.commonSlice.Close()
		o.commonSlice = ds
		o.spilling = true
		v.Reset()
		v.Unmarshal(d)
	}
	o.append(v)
}

func (o *Slice) released() {
	o.iterating = false
}

func (o *Slice) NewIterator(reverse bool) ValueIterator {
	if o.iterating {
		panic(concurrencyMsg)
	}
	o.iterating = true
	return o.newIterator(o, reverse)
}

// Close is just here to catch deferred calls to Close, such that the correct
// method is called in case spilling happened.
func (o *Slice) Close() {
	o.commonSlice.Close()
}

func (o *Slice) String() string {
	return fmt.Sprintf("Slice@%p", o)
}

type memorySlice struct {
	values []Value
	bytes  int
}

func (o *memorySlice) append(v Value) {
	o.values = append(o.values, v)
	o.bytes += v.Bytes()
}

func (o *memorySlice) Bytes() int {
	return o.bytes
}

func (o *memorySlice) Close() {
	o.values = nil
}

func (o *memorySlice) newIterator(p iteratorParent, reverse bool) ValueIterator {
	return newMemIterator(o.values, p, reverse, len(o.values))
}

func (o *memorySlice) Items() int {
	return len(o.values)
}

type diskSlice struct {
	*diskSorted
}

func (o *diskSlice) append(v Value) {
	o.diskSorted.add(nil, v)
}

func (o *diskSlice) newIterator(p iteratorParent, reverse bool) ValueIterator {
	return o.diskSorted.newIterator(p, reverse)
}

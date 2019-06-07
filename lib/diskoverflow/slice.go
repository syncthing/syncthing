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

type Slice interface {
	Common
	Append(v Value) error
	NewIterator() Iterator
	NewReverseIterator() Iterator
}

type slice struct {
	commonSlice
	base
}

type commonSlice interface {
	common
	append(v Value) error
	newIterator(reverse bool) Iterator
}

// NewSorted creates a slice like container, spilling to disk at location.
// All items added to this instance must be of the same type as v.
func NewSlice(location string) Slice {
	o := &slice{base: newBase(location)}
	o.commonSlice = &memSlice{}
	return o
}

func (o *slice) Append(v Value) error {
	if o.startSpilling(o.Bytes() + v.ProtoSize()) {
		d, err := v.Marshal()
		if err != nil {
			return err
		}
		newMap, err := newDiskMap(o.location)
		if err != nil {
			return err
		}
		ds := &diskSlice{newMap}
		it := o.NewIterator()
		for it.Next() {
			v.Reset()
			if err := it.Value(v); err != nil {
				return err
			}
			if err := ds.append(v); err != nil {
				return err
			}
		}
		it.Release()
		o.commonSlice.Close()
		o.commonSlice = ds
		o.spilling = true
		v.Reset()
		if err := v.Unmarshal(d); err != nil {
			return err
		}
	}
	return o.append(v)
}

func (o *slice) NewIterator() Iterator {
	return o.newIterator(false)
}

func (o *slice) NewReverseIterator() Iterator {
	return o.newIterator(true)
}

// Close is just here to catch deferred calls to Close, such that the correct
// method is called in case spilling happened.
func (o *slice) Close() {
	o.commonSlice.Close()
}

func (o *slice) String() string {
	return fmt.Sprintf("Slice@%p", o)
}

type memSlice struct {
	values []Value
	bytes  int
}

func (o *memSlice) append(v Value) error {
	o.values = append(o.values, v)
	o.bytes += v.ProtoSize()
	return nil
}

func (o *memSlice) Bytes() int {
	return o.bytes
}

func (o *memSlice) Close() {
	o.values = nil
}

type sliceIterator struct {
	*posIterator
	values []Value
}

func (o *memSlice) newIterator(reverse bool) Iterator {
	return &sliceIterator{
		posIterator: newPosIterator(len(o.values), reverse),
		values:      o.values,
	}
}

func (si *sliceIterator) Value(v Value) error {
	if si.offset < si.len {
		copyValue(v, si.values[si.pos()])
	}
	return nil
}

func (o *memSlice) Items() int {
	return len(o.values)
}

type diskSlice struct {
	*diskMap
}

const indexLength = 8

func (o *diskSlice) append(v Value) error {
	index := make([]byte, indexLength)
	binary.BigEndian.PutUint64(index, uint64(o.Items()))
	return o.diskMap.set(index, v)
}

func (o *diskSlice) newIterator(reverse bool) Iterator {
	return o.diskMap.newIterator(reverse)
}

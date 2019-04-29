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
	base
	v Value
}

type commonSlice interface {
	common
	append(v Value)
	newIterator(p iteratorParent, reverse bool) ValueIterator
}

func NewSlice(location string, v Value) *Slice {
	o := &Slice{
		base: newBase(location),
		v:    v,
	}
	o.commonSlice = &memorySlice{}
	return o
}

func (o *Slice) Append(v Value) {
	if o.iterating {
		panic(concurrencyMsg)
	}
	if o.startSpilling(o.Bytes() + v.Bytes()) {
		ds := &diskSlice{newDiskSorted(o.location, &nonSortValue{Value: o.v})}
		it := o.newIterator(o, false)
		for it.Next() {
			ds.append(it.Value())
		}
		it.Release()
		o.commonSlice.Close()
		o.commonSlice = ds
		o.spilling = true
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

func (o *Slice) String() string {
	return fmt.Sprintf("Slice@%p", o)
}

func (o *Slice) value() interface{} {
	return o.v
}

type memorySlice struct {
	values []Value
	bytes  int64
}

func (o *memorySlice) append(v Value) {
	o.values = append(o.values, v)
	o.bytes += v.Bytes()
}

func (o *memorySlice) Bytes() int64 {
	return o.bytes
}

func (o *memorySlice) Close() {
	o.values = nil
}

type memValueIterator struct {
	*memIterator
	values []Value
}

func (o *memorySlice) newIterator(p iteratorParent, reverse bool) ValueIterator {
	return &memValueIterator{
		memIterator: newMemIterator(p, reverse, len(o.values)),
		values:      o.values,
	}
}

func (si *memValueIterator) Value() Value {
	if si.pos == si.len || si.pos == -1 {
		return nil
	}
	return si.values[si.pos]
}

func (o *memorySlice) Items() int {
	return len(o.values)
}

type diskSlice struct {
	*diskSorted
}

func (o *diskSlice) append(v Value) {
	o.diskSorted.add(&nonSortValue{v, uint64(o.len)})
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

func (o *diskSlice) newIterator(p iteratorParent, reverse bool) ValueIterator {
	return &diskSliceIterator{o.diskSorted.newIterator(p, reverse)}
}

// nonSortValue implements the SortValue interface, to be "sorted" by insertion order.
type nonSortValue struct {
	Value
	index uint64
}

func (o *nonSortValue) Key() []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key[:], uint64(o.index))
	return key
}

func (o *nonSortValue) UnmarshalWithKey(key, value []byte) SortValue {
	return &nonSortValue{
		Value: o.Value.Unmarshal(value),
		index: binary.BigEndian.Uint64(key),
	}
}

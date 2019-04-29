// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package diskoverflow

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/syndtr/goleveldb/leveldb/iterator"
)

const suffixLength = 8

// SortValue must be implemented by every supported type for sorting. The sorting
// will happen according to bytes.Compare on the key.
type SortValue interface {
	Value
	UnmarshalWithKey(key, value []byte) SortValue // The returned SortValue must not be a reference to the receiver.
	Key() []byte
}

type Sorted struct {
	commonSorted
	base
	v SortValue
}

type commonSorted interface {
	common
	add(v SortValue)
	getFirst() (SortValue, bool)
	getLast() (SortValue, bool)
	dropFirst(v SortValue) bool
	dropLast(v SortValue) bool
	newIterator(p iteratorParent, reverse bool) SortValueIterator
}

func NewSorted(location string, v SortValue) *Sorted {
	o := &Sorted{
		base: newBase(location),
		v:    v,
	}
	o.commonSorted = &memorySorted{}
	return o
}

func (o *Sorted) Add(v SortValue) {
	if o.startSpilling(o.Bytes() + v.Bytes()) {
		newSorted := newDiskSorted(o.location, o.v)
		it := o.NewIterator(false)
		for it.Next() {
			newSorted.add(it.Value())
		}
		it.Release()
		o.commonSorted.Close()
		o.commonSorted = newSorted
		o.spilling = true
	}
	o.add(v)
}

func (o *Sorted) NewIterator(reverse bool) SortValueIterator {
	if o.iterating {
		panic(concurrencyMsg)
	}
	o.iterating = true
	return o.newIterator(o, reverse)
}

func (o *Sorted) PopFirst() (SortValue, bool) {
	v, ok := o.getFirst()
	if ok {
		o.dropFirst(v)
	}
	return v, ok
}

func (o *Sorted) PopLast() (SortValue, bool) {
	v, ok := o.getLast()
	if ok {
		o.dropLast(v)
	}
	return v, ok
}

func (o *Sorted) String() string {
	return fmt.Sprintf("Sorted@%p", o)
}

func (o *Sorted) value() interface{} {
	return o.v
}

func (o *Sorted) released() {
	o.iterating = false
}

// memorySorted is basically a slice that keeps track of its size and supports
// sorted iteration of its element.
type memorySorted struct {
	bytes    int64
	outgoing bool
	values   sortSlice
}

func (o *memorySorted) add(v SortValue) {
	if o.outgoing {
		panic("Add/Append may never be called after PopFirst/PopLast")
	}
	o.values = append(o.values, v)
	o.bytes += v.Bytes()
}

type memSortValueIterator struct {
	*memIterator
	values []SortValue
}

func (si *memSortValueIterator) Value() SortValue {
	if si.pos == si.len || si.pos == -1 {
		return nil
	}
	return si.values[si.pos]
}

func (o *memorySorted) newIterator(parent iteratorParent, reverse bool) SortValueIterator {
	if !o.outgoing {
		sort.Sort(o.values)
	}

	return &memSortValueIterator{
		memIterator: newMemIterator(parent, reverse, len(o.values)),
		values:      o.values,
	}
}

func (o *memorySorted) Bytes() int64 {
	return o.bytes
}

func (o *memorySorted) Close() {
}

func (o *memorySorted) Items() int {
	return len(o.values)
}

func (o *memorySorted) getFirst() (SortValue, bool) {
	if !o.outgoing {
		sort.Sort(o.values)
		o.outgoing = true
	}

	if o.Items() == 0 {
		return nil, false
	}
	return o.values[0], true
}

func (o *memorySorted) getLast() (SortValue, bool) {
	if !o.outgoing {
		sort.Sort(o.values)
		o.outgoing = true
	}

	if o.Items() == 0 {
		return nil, false
	}
	return o.values[o.Items()-1], true
}

func (o *memorySorted) dropFirst(v SortValue) bool {
	if len(o.values) == 0 {
		return false
	}
	o.values = o.values[1:]
	o.bytes -= v.Bytes()
	return true
}

func (o *memorySorted) dropLast(v SortValue) bool {
	if len(o.values) == 0 {
		return false
	}
	o.values = o.values[:len(o.values)-1]
	o.bytes -= v.Bytes()
	return true
}

// diskSorted is backed by a LevelDB database in a temporary directory. It relies
// on the fact that iterating over the database is done in key order.
type diskSorted struct {
	*diskMap
	bytes int64
	v     SortValue
}

func newDiskSorted(loc string, v SortValue) *diskSorted {
	return &diskSorted{
		diskMap: newDiskMap(loc, v),
		v:       v,
	}
}

func (d *diskSorted) add(v SortValue) {
	suffix := make([]byte, suffixLength)
	binary.BigEndian.PutUint64(suffix[:], uint64(d.len))
	d.diskMap.addBytes(append(v.Key(), suffix...), v)
	d.bytes += v.Bytes()
}

func (d *diskSorted) Bytes() int64 {
	return d.bytes
}

type diskIterator struct {
	it     iterator.Iterator
	v      SortValue
	parent iteratorParent
}

func (di *diskIterator) Value() SortValue {
	key := di.it.Key()
	if key == nil {
		return nil
	}
	return di.v.UnmarshalWithKey(key[:len(key)-suffixLength], di.it.Value())
}

func (di *diskIterator) Release() {
	di.it.Release()
	di.parent.released()
}

type diskForwardIterator struct {
	*diskIterator
}

func (i *diskForwardIterator) Next() bool {
	return i.it.Next()
}

type diskReverseIterator struct {
	*diskIterator
	next func(*diskReverseIterator) bool
}

func (i *diskReverseIterator) Next() bool {
	return i.next(i)
}

func (d *diskSorted) newIterator(parent iteratorParent, reverse bool) SortValueIterator {
	di := &diskIterator{
		it:     d.db.NewIterator(nil, nil),
		v:      d.v,
		parent: parent,
	}
	if reverse {
		ri := &diskReverseIterator{diskIterator: di}
		ri.next = func(i *diskReverseIterator) bool {
			i.next = func(j *diskReverseIterator) bool {
				return j.it.Prev()
			}
			return i.it.Last()
		}
		return ri
	}
	return &diskForwardIterator{di}
}

func (d *diskSorted) getFirst() (SortValue, bool) {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()
	if !it.First() {
		return nil, false
	}
	key := it.Key()
	return d.v.UnmarshalWithKey(key[:len(key)-suffixLength], it.Value()), true
}

func (d *diskSorted) getLast() (SortValue, bool) {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()
	if !it.Last() {
		return nil, false
	}
	key := it.Key()
	return d.v.UnmarshalWithKey(key[:len(key)-suffixLength], it.Value()), true
}

func (d *diskSorted) dropFirst(v SortValue) bool {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()
	if !it.First() {
		return false
	}
	d.db.Delete(it.Key(), nil)
	d.bytes -= v.Bytes()
	d.len--
	return true
}

func (d *diskSorted) dropLast(v SortValue) bool {
	it := d.db.NewIterator(nil, nil)
	defer it.Release()
	if !it.Last() {
		return false
	}
	d.db.Delete(it.Key(), nil)
	d.bytes -= v.Bytes()
	d.len--
	return true
}

// sortSlice is a sortable slice of sortValues
type sortSlice []SortValue

func (s sortSlice) Len() int {
	return len(s)
}
func (s sortSlice) Swap(a, b int) {
	s[a], s[b] = s[b], s[a]
}
func (s sortSlice) Less(a, b int) bool {
	return bytes.Compare(s[a].Key(), s[b].Key()) == -1
}

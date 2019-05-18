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
)

const suffixLength = 8

// Sorted is a disk-spilling container, that sorts added values by the
// accompanying keys.
// You must not add new values after calling PopFirst/-Last.
type Sorted interface {
	Add(k []byte, v Value)
	PopFirst(v Value) bool
	PopLast(v Value) bool
	NewIterator() Iterator
	NewReverseIterator() Iterator
	Bytes() int
	Items() int
	SetOverflowBytes(bytes int)
	Close()
}

type sorted struct {
	commonSorted
	base
}

type keyIterator interface {
	Iterator
	key() []byte
}

type commonSorted interface {
	common
	add(k []byte, v Value)
	PopFirst(v Value) bool
	PopLast(v Value) bool
	newIterator(p iteratorParent, reverse bool) keyIterator
}

// NewSorted returns an implementaiton of Sorted, spilling to disk at location.
func NewSorted(location string) Sorted {
	o := &sorted{base: newBase(location)}
	o.commonSorted = &memorySorted{}
	return o
}

func (o *sorted) Add(k []byte, v Value) {
	if o.startSpilling(o.Bytes() + v.Bytes()) {
		d := v.Marshal()
		newSorted := newDiskSorted(o.location)
		it := o.NewIterator().(keyIterator)
		for it.Next() {
			v.Reset()
			it.Value(v)
			newSorted.add(it.key(), v)
		}
		it.Release()
		o.commonSorted.Close()
		o.commonSorted = newSorted
		o.spilling = true
		v.Reset()
		v.Unmarshal(d)
	}
	o.add(k, v)
}

func (o *sorted) NewIterator() Iterator {
	return o.newIterator(false)
}

func (o *sorted) NewReverseIterator() Iterator {
	return o.newIterator(true)
}

func (o *sorted) newIterator(reverse bool) Iterator {
	if o.iterating {
		panic(concurrencyMsg)
	}
	o.iterating = true
	return o.commonSorted.newIterator(o, reverse)
}

// Close is just here to catch deferred calls to Close, such that the correct
// method is called in case spilling happened.
func (o *sorted) Close() {
	o.commonSorted.Close()
}

func (o *sorted) String() string {
	return fmt.Sprintf("Sorted@%p", o)
}

func (o *sorted) released() {
	o.iterating = false
}

// memorySorted is basically a slice that keeps track of its size and supports
// sorted iteration of its element.
type memorySorted struct {
	bytes    int
	outgoing bool
	values   []Value
	keys     [][]byte
}

func (o *memorySorted) add(k []byte, v Value) {
	if o.outgoing {
		panic("Add/Append may never be called after PopFirst/PopLast")
	}
	o.values = append(o.values, v)
	o.keys = append(o.keys, k)
	o.bytes += v.Bytes()
}

func (o *memorySorted) Len() int {
	return len(o.values)
}
func (o *memorySorted) Swap(a, b int) {
	o.values[a], o.values[b] = o.values[b], o.values[a]
	o.keys[a], o.keys[b] = o.keys[b], o.keys[a]
}
func (o *memorySorted) Less(a, b int) bool {
	return bytes.Compare(o.keys[a], o.keys[b]) == -1
}

func (o *memorySorted) Bytes() int {
	return o.bytes
}

func (o *memorySorted) Close() {}

func (o *memorySorted) Items() int {
	return len(o.values)
}

func (o *memorySorted) PopFirst(v Value) bool {
	return o.pop(v, true)
}

func (o *memorySorted) PopLast(v Value) bool {
	return o.pop(v, false)
}

func (o *memorySorted) pop(v Value, first bool) bool {
	if !o.outgoing {
		sort.Sort(o)
		o.outgoing = true
	}
	if o.Items() == 0 {
		return false
	}
	if first {
		copyValue(v, o.values[0])
		o.values = o.values[1:]
		o.keys = o.keys[1:]
	} else {
		i := o.Items() - 1
		copyValue(v, o.values[i])
		o.values = o.values[:i]
		o.keys = o.keys[:i]
	}
	o.bytes -= v.Bytes()
	return true
}

func (o *memorySorted) newIterator(parent iteratorParent, reverse bool) keyIterator {
	if !o.outgoing {
		sort.Sort(o)
	}
	return &memSortIterator{
		memIterator: newMemIterator(o.values, parent, reverse, o.Items()),
		keys:        o.keys,
	}
}

type memSortIterator struct {
	*memIterator
	keys [][]byte
}

func (si *memSortIterator) key() []byte {
	if si.pos == si.len || si.pos == -1 {
		return nil
	}
	return si.keys[si.pos]
}

// diskSorted is backed by a LevelDB database in a temporary directory. It relies
// on the fact that iterating over the database is done in key order.
type diskSorted struct {
	*diskMap
	bytes int
}

func newDiskSorted(loc string) *diskSorted {
	return &diskSorted{diskMap: newDiskMap(loc)}
}

func (o *diskSorted) add(k []byte, v Value) {
	suffix := make([]byte, suffixLength)
	binary.BigEndian.PutUint64(suffix[:], uint64(o.Items()))
	o.diskMap.addBytes(append(k, suffix...), v)
	o.bytes += v.Bytes()
}

func (o *diskSorted) Bytes() int {
	return o.bytes
}

func (o *diskSorted) PopFirst(v Value) bool {
	return o.pop(v, true)
}

func (o *diskSorted) PopLast(v Value) bool {
	return o.pop(v, false)
}

func (o *diskSorted) pop(v Value, first bool) bool {
	it := o.db.NewIterator(nil, nil)
	defer it.Release()
	var ok bool
	if first {
		ok = it.First()
	} else {
		ok = it.Last()
	}
	if !ok {
		return false
	}
	v.Unmarshal(it.Value())
	_ = o.db.Delete(it.Key(), nil)
	o.bytes -= v.Bytes()
	o.len--
	return true
}

func (o *diskSorted) newIterator(parent iteratorParent, reverse bool) keyIterator {
	di := &diskIterator{
		it:     o.db.NewIterator(nil, nil),
		parent: parent,
	}
	if !reverse {
		return di
	}
	ri := &diskReverseIterator{diskIterator: di}
	ri.next = func(i *diskReverseIterator) bool {
		i.next = func(j *diskReverseIterator) bool {
			return j.it.Prev()
		}
		return i.it.Last()
	}
	return ri
}

type diskReverseIterator struct {
	*diskIterator
	next func(*diskReverseIterator) bool
}

func (i *diskReverseIterator) Next() bool {
	return i.next(i)
}

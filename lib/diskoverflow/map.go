// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package diskoverflow

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

type Map struct {
	commonMap
	base
}

type commonMap interface {
	common
	add(k string, v Value)
	Get(k string, v Value) bool
	Pop(k string, v Value) bool
	Delete(k string)
	newIterator(p iteratorParent) MapIterator
}

// NewSorted creates a map like container, spilling to disk at location.
// All items added to this instance must be of the same type as v.
func NewMap(location string) *Map {
	o := &Map{base: newBase(location)}
	o.commonMap = &memoryMap{
		values: make(map[string]Value),
	}
	return o
}

func (o *Map) Add(k string, v Value) {
	if o.iterating {
		panic(concurrencyMsg)
	}
	if o.startSpilling(o.Bytes() + v.Bytes()) {
		d := v.Marshal()
		newMap := newDiskMap(o.location)
		it := o.newIterator(o)
		for it.Next() {
			v.Reset()
			it.Value(v)
			newMap.add(it.Key(), v)
		}
		it.Release()
		o.commonMap.Close()
		o.commonMap = newMap
		o.spilling = true
		v.Reset()
		v.Unmarshal(d)
	}
	o.add(k, v)
}

func (o *Map) String() string {
	return fmt.Sprintf("Map@%p", o)
}

// Close is just here to catch deferred calls to Close, such that the correct
// method is called in case spilling happened.
func (o *Map) Close() {
	o.commonMap.Close()
}

func (o *Map) released() {
	o.iterating = false
}

type MapIterator interface {
	ValueIterator
	Key() string
}

func (o *Map) NewIterator() MapIterator {
	if o.iterating {
		panic(concurrencyMsg)
	}
	return o.newIterator(o)
}

type memoryMap struct {
	values map[string]Value
	bytes  int
}

func (o *memoryMap) add(k string, v Value) {
	o.values[k] = v
	o.bytes += v.Bytes()
}

func (o *memoryMap) Bytes() int {
	return o.bytes
}

func (o *memoryMap) Close() {
	o.values = nil
}

func (o *memoryMap) Get(k string, v Value) bool {
	nv, ok := o.values[k]
	if !ok {
		return false
	}
	v.Copy(nv)
	return true
}

func (o *memoryMap) Items() int {
	return len(o.values)
}

func (o *memoryMap) Pop(k string, v Value) bool {
	ok := o.Get(k, v)
	if !ok {
		return false
	}
	delete(o.values, k)
	o.bytes -= v.Bytes()
	return true
}

func (o *memoryMap) Delete(k string) {
	v, ok := o.values[k]
	if !ok {
		return
	}
	delete(o.values, k)
	o.bytes -= v.Bytes()
}

type iteratorValue struct {
	k string
	v Value
}

type memMapIterator struct {
	values  map[string]Value
	ch      chan iteratorValue
	stop    chan struct{}
	current iteratorValue
	parent  iteratorParent
}

func (o *memoryMap) newIterator(p iteratorParent) MapIterator {
	i := &memMapIterator{
		ch:     make(chan iteratorValue),
		stop:   make(chan struct{}),
		parent: p,
		values: o.values,
	}
	go i.iterate()
	return i
}

func (i *memMapIterator) iterate() {
	for k, v := range i.values {
		select {
		case <-i.stop:
			break
		case i.ch <- iteratorValue{k, v}:
		}
	}
	close(i.ch)
}

func (i *memMapIterator) Key() string {
	return i.current.k
}

func (i *memMapIterator) Value(v Value) {
	v.Copy(i.current.v)
}

func (i *memMapIterator) Next() bool {
	var ok bool
	i.current, ok = <-i.ch
	return ok
}

func (i *memMapIterator) Release() {
	close(i.stop)
	i.parent.released()
}

type diskMap struct {
	db    *leveldb.DB
	bytes int
	dir   string
	len   int
}

func newDiskMap(location string) *diskMap {
	// Use a temporary database directory.
	tmp, err := ioutil.TempDir(location, "overflow-")
	if err != nil {
		panic("creating temporary directory: " + err.Error())
	}
	db, err := leveldb.OpenFile(tmp, &opt.Options{
		OpenFilesCacheCapacity: 10,
		WriteBuffer:            512 << 10,
	})
	if err != nil {
		panic("creating temporary database: " + err.Error())
	}
	return &diskMap{
		db:  db,
		dir: tmp,
	}
}

func (o *diskMap) add(k string, v Value) {
	o.addBytes([]byte(k), v)
	o.bytes += v.Bytes()
}

func (o *diskMap) addBytes(k []byte, v Value) {
	if err := o.db.Put(k, v.Marshal(), nil); err != nil {
		panic("writing to temporary database: " + err.Error())
	}
	o.len++
}

func (o *diskMap) Close() {
	o.db.Close()
	os.RemoveAll(o.dir)
}

func (o *diskMap) Bytes() int {
	return o.bytes
}

func (o *diskMap) Get(k string, v Value) bool {
	d, err := o.db.Get([]byte(k), nil)
	if err != nil {
		return false
	}
	v.Unmarshal(d)
	return true
}

func (o *diskMap) Items() int {
	return o.len
}

func (o *diskMap) Pop(k string, v Value) bool {
	ok := o.Get(k, v)
	if ok {
		o.db.Delete([]byte(k), nil)
		o.len--
	}
	return ok
}

func (o *diskMap) Delete(k string) {
	_, err := o.db.Get([]byte(k), nil)
	if err == nil {
		o.db.Delete([]byte(k), nil)
		o.len--
	}
}

func (o *diskMap) newIterator(p iteratorParent) MapIterator {
	return &diskIterator{
		it:     o.db.NewIterator(nil, nil),
		parent: p,
	}
}

type diskIterator struct {
	it     iterator.Iterator
	parent iteratorParent
}

func (i *diskIterator) Next() bool {
	return i.it.Next()
}

func (i *diskIterator) Value(v Value) {
	v.Unmarshal(i.it.Value())
}

func (i *diskIterator) key() []byte {
	return i.it.Key()
}

func (i *diskIterator) Key() string {
	return string(i.key())
}

func (i *diskIterator) Release() {
	i.it.Release()
	i.parent.released()
}

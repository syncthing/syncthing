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
	v Value
}

type commonMap interface {
	common
	add(k string, v Value)
	Get(k string) (Value, bool)
	Pop(k string) (Value, bool)
	newIterator(p iteratorParent) MapIterator
}

func NewMap(location string, v Value) *Map {
	o := &Map{
		base: newBase(location),
		v:    v,
	}
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
		newMap := newDiskMap(o.location, o.v)
		it := o.newIterator(o)
		for it.Next() {
			newMap.add(it.Key(), it.Value())
		}
		it.Release()
		o.commonMap.Close()
		o.commonMap = newMap
		o.spilling = true
	}
	o.add(k, v)
}

func (o *Map) String() string {
	return fmt.Sprintf("Map@%p", o)
}

func (o *Map) released() {
	o.iterating = false
}

func (o *Map) value() interface{} {
	return o.v
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
	bytes  int64
}

func (o *memoryMap) add(k string, v Value) {
	o.bytes += v.Bytes()
	o.values[k] = v
}

func (o *memoryMap) Bytes() int64 {
	return o.bytes
}

func (o *memoryMap) Close() {
	o.values = nil
}

func (o *memoryMap) Get(key string) (Value, bool) {
	v, ok := o.values[key]
	return v, ok
}

func (o *memoryMap) Items() int {
	return len(o.values)
}

func (o *memoryMap) Pop(key string) (Value, bool) {
	v, ok := o.values[key]
	if !ok {
		return nil, false
	}
	delete(o.values, key)
	o.bytes -= v.Bytes()
	return v, ok
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

func (i *memMapIterator) Value() Value {
	return i.current.v
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
	bytes int64
	dir   string
	len   int
	v     Value
}

func newDiskMap(location string, v Value) *diskMap {
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
		v:   v,
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

func (o *diskMap) Bytes() int64 {
	return o.bytes
}

func (o *diskMap) Get(k string) (Value, bool) {
	data, err := o.db.Get([]byte(k), nil)
	if err != nil {
		return nil, false
	}
	return o.v.Unmarshal(data), true
}

func (o *diskMap) Items() int {
	return o.len
}

func (o *diskMap) Pop(k string) (Value, bool) {
	v, ok := o.Get(k)
	if ok {
		o.db.Delete([]byte(k), nil)
		o.len--
	}
	return v, ok
}

func (o *diskMap) newIterator(p iteratorParent) MapIterator {
	return &diskMapIterator{
		it:     o.db.NewIterator(nil, nil),
		v:      p.value().(Value),
		parent: p,
	}
}

type diskMapIterator struct {
	it     iterator.Iterator
	v      Value
	parent iteratorParent
}

func (i *diskMapIterator) Next() bool {
	return i.it.Next()
}

func (i *diskMapIterator) Value() Value {
	return i.v.Unmarshal(i.it.Value())
}

func (i *diskMapIterator) Key() string {
	return string(i.it.Key())
}

func (i *diskMapIterator) Release() {
	i.it.Release()
	i.parent.released()
}

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

type Map interface {
	Common
	Set(k []byte, v Value)
	Get(k []byte, v Value) bool
	Pop(k []byte, v Value) bool
	Delete(k []byte)
	NewIterator() MapIterator
}

type omap struct {
	commonMap
	base
}

type veryCommonMap interface {
	common
	set(k []byte, v Value)
	Get(k []byte, v Value) bool
	Pop(k []byte, v Value) bool
	Delete(k []byte)
}

type commonMap interface {
	veryCommonMap
	NewIterator() MapIterator
}

// NewMap returns an implementation of Map, spilling to disk at location.
func NewMap(location string) Map {
	o := &omap{base: newBase(location)}
	o.commonMap = &memMap{
		values: make(map[string]Value),
	}
	return o
}

func (o *omap) Set(k []byte, v Value) {
	if o.startSpilling(o.Bytes() + v.ProtoSize()) {
		d, err := v.Marshal()
		errPanic(err)
		newMap := newDiskMap(o.location)
		it := o.NewIterator()
		for it.Next() {
			v.Reset()
			it.Value(v)
			newMap.set(it.Key(), v)
		}
		it.Release()
		o.commonMap.Close()
		o.commonMap = newMap
		o.spilling = true
		v.Reset()
		errPanic(v.Unmarshal(d))
	}
	o.set(k, v)
}

func (o *omap) String() string {
	return fmt.Sprintf("Map@%p", o)
}

// Close is just here to catch deferred calls to Close, such that the correct
// method is called in case spilling happened.
func (o *omap) Close() {
	o.commonMap.Close()
}

type MapIterator interface {
	Iterator
	Key() []byte
}

type memMap struct {
	values map[string]Value
	bytes  int
}

func (o *memMap) set(k []byte, v Value) {
	s := string(k)
	if ov, ok := o.values[s]; ok {
		o.bytes -= ov.ProtoSize()
	}
	o.values[s] = v
	o.bytes += v.ProtoSize()
}

func (o *memMap) Bytes() int {
	return o.bytes
}

func (o *memMap) Close() {}

func (o *memMap) Get(k []byte, v Value) bool {
	nv, ok := o.values[string(k)]
	if !ok {
		return false
	}
	copyValue(v, nv)
	return true
}

func (o *memMap) Items() int {
	return len(o.values)
}

func (o *memMap) Pop(k []byte, v Value) bool {
	ok := o.Get(k, v)
	if !ok {
		return false
	}
	delete(o.values, string(k))
	o.bytes -= v.ProtoSize()
	return true
}

func (o *memMap) Delete(k []byte) {
	s := string(k)
	v, ok := o.values[s]
	if !ok {
		return
	}
	delete(o.values, s)
	o.bytes -= v.ProtoSize()
}

type iteratorValue struct {
	k []byte
	v Value
}

type memMapIterator struct {
	values  map[string]Value
	ch      chan iteratorValue
	stop    chan struct{}
	current iteratorValue
}

func (o *memMap) NewIterator() MapIterator {
	i := &memMapIterator{
		ch:     make(chan iteratorValue),
		stop:   make(chan struct{}),
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
		case i.ch <- iteratorValue{[]byte(k), v}:
		}
	}
	close(i.ch)
}

func (i *memMapIterator) Key() []byte {
	return i.current.k
}

func (i *memMapIterator) Value(v Value) {
	copyValue(v, i.current.v)
}

func (i *memMapIterator) Next() bool {
	var ok bool
	i.current, ok = <-i.ch
	return ok
}

func (i *memMapIterator) Release() {
	close(i.stop)
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

func (o *diskMap) set(k []byte, v Value) {
	old, oldErr := o.db.Get([]byte(k), nil)
	d, err := v.Marshal()
	errPanic(err)
	errPanic(o.db.Put(k, d, nil))
	o.len++
	o.bytes += v.ProtoSize()
	if oldErr == nil {
		errPanic(v.Unmarshal(old))
		o.bytes -= v.ProtoSize()
	}
}

func (o *diskMap) Close() {
	o.db.Close()
	os.RemoveAll(o.dir)
}

func (o *diskMap) Bytes() int {
	return o.bytes
}

func (o *diskMap) Get(k []byte, v Value) bool {
	d, err := o.db.Get([]byte(k), nil)
	if err != nil {
		return false
	}
	errPanic(v.Unmarshal(d))
	return true
}

func (o *diskMap) Items() int {
	return o.len
}

func (o *diskMap) Pop(k []byte, v Value) bool {
	ok := o.Get(k, v)
	if ok {
		errPanic(o.db.Delete([]byte(k), nil))
		o.len--
	}
	return ok
}

func (o *diskMap) PopFirst(v Value) bool {
	return o.pop(v, true)
}

func (o *diskMap) PopLast(v Value) bool {
	return o.pop(v, false)
}

func (o *diskMap) pop(v Value, first bool) bool {
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
	errPanic(v.Unmarshal(it.Value()))
	errPanic(o.db.Delete(it.Key(), nil))
	o.bytes -= v.ProtoSize()
	o.len--
	return true
}

func (o *diskMap) Delete(k []byte) {
	errPanic(o.db.Delete([]byte(k), nil))
	o.len--
}

func (o *diskMap) NewIterator() MapIterator {
	return o.newIterator(false)
}

func (o *diskMap) newIterator(reverse bool) MapIterator {
	di := &diskIterator{o.db.NewIterator(nil, nil)}
	if !reverse {
		return di
	}
	ri := &diskReverseIterator{diskIterator: di}
	ri.next = func(i *diskReverseIterator) bool {
		i.next = func(j *diskReverseIterator) bool {
			return j.Prev()
		}
		return i.Last()
	}
	return ri
}

type diskIterator struct {
	iterator.Iterator
}

func (i *diskIterator) Value(v Value) {
	errPanic(v.Unmarshal(i.Iterator.Value()))
}

type diskReverseIterator struct {
	*diskIterator
	next func(*diskReverseIterator) bool
}

func (i *diskReverseIterator) Next() bool {
	return i.next(i)
}

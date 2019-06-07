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
	Set(k []byte, v Value) error
	Get(k []byte, v Value) (bool, error)
	Pop(k []byte, v Value) (bool, error)
	Delete(k []byte) error
	NewIterator() MapIterator
}

type omap struct {
	commonMap
	base
}

type veryCommonMap interface {
	common
	set(k []byte, v Value) error
	Get(k []byte, v Value) (bool, error)
	Pop(k []byte, v Value) (bool, error)
	Delete(k []byte) error
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

func (o *omap) Set(k []byte, v Value) error {
	if o.startSpilling(o.Bytes() + v.ProtoSize()) {
		d, err := v.Marshal()
		if err != nil {
			return err
		}
		newMap, err := newDiskMap(o.location)
		if err != nil {
			return err
		}
		it := o.NewIterator()
		for it.Next() {
			v.Reset()
			if err := it.Value(v); err != nil {
				return err
			}
			err = newMap.set(it.Key(), v)
			if err != nil {
				return err
			}
		}
		it.Release()
		o.commonMap.Close()
		o.commonMap = newMap
		o.spilling = true
		v.Reset()
		if err := v.Unmarshal(d); err != nil {
			return err
		}
	}
	return o.set(k, v)
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

func (o *memMap) set(k []byte, v Value) error {
	s := string(k)
	if ov, ok := o.values[s]; ok {
		o.bytes -= ov.ProtoSize()
	}
	o.values[s] = v
	o.bytes += v.ProtoSize()
	return nil
}

func (o *memMap) Bytes() int {
	return o.bytes
}

func (o *memMap) Close() {}

func (o *memMap) Get(k []byte, v Value) (bool, error) {
	nv, ok := o.values[string(k)]
	if !ok {
		return false, nil
	}
	copyValue(v, nv)
	return true, nil
}

func (o *memMap) Items() int {
	return len(o.values)
}

func (o *memMap) Pop(k []byte, v Value) (bool, error) {
	ok, err := o.Get(k, v)
	if !ok || err != nil {
		return false, err
	}
	delete(o.values, string(k))
	o.bytes -= v.ProtoSize()
	return true, nil
}

func (o *memMap) Delete(k []byte) error {
	s := string(k)
	v, ok := o.values[s]
	if !ok {
		return nil
	}
	delete(o.values, s)
	o.bytes -= v.ProtoSize()
	return nil
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

func (i *memMapIterator) Value(v Value) error {
	copyValue(v, i.current.v)
	return nil
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

func newDiskMap(location string) (*diskMap, error) {
	// Use a temporary database directory.
	tmp, err := ioutil.TempDir(location, "overflow-")
	if err != nil {
		return nil, err
	}
	db, err := leveldb.OpenFile(tmp, &opt.Options{
		OpenFilesCacheCapacity: 10,
		WriteBuffer:            512 << 10,
	})
	if err != nil {
		return nil, err
	}
	return &diskMap{
		db:  db,
		dir: tmp,
	}, nil
}

func (o *diskMap) set(k []byte, v Value) error {
	old, oldErr := o.db.Get(k, nil)
	d, err := v.Marshal()
	if err != nil {
		return err
	}
	if err := o.db.Put(k, d, nil); err != nil {
		return err
	}
	o.len++
	o.bytes += v.ProtoSize()
	if oldErr == nil {
		if err := v.Unmarshal(old); err != nil {
			return err
		}
		o.bytes -= v.ProtoSize()
	}
	return nil
}

func (o *diskMap) Close() {
	o.db.Close()
	os.RemoveAll(o.dir)
}

func (o *diskMap) Bytes() int {
	return o.bytes
}

func (o *diskMap) Get(k []byte, v Value) (bool, error) {
	d, err := o.db.Get(k, nil)
	if err != nil {
		return false, nil
	}
	if err := v.Unmarshal(d); err != nil {
		return false, err
	}
	return true, nil
}

func (o *diskMap) Items() int {
	return o.len
}

func (o *diskMap) Pop(k []byte, v Value) (bool, error) {
	ok, err := o.Get(k, v)
	if err != nil {
		return false, err
	}
	if ok {
		if err := o.db.Delete(k, nil); err != nil {
			return false, err
		}
		o.len--
	}
	return ok, nil
}

func (o *diskMap) PopFirst(v Value) (bool, error) {
	return o.pop(v, true)
}

func (o *diskMap) PopLast(v Value) (bool, error) {
	return o.pop(v, false)
}

func (o *diskMap) pop(v Value, first bool) (bool, error) {
	it := o.db.NewIterator(nil, nil)
	defer it.Release()
	var ok bool
	if first {
		ok = it.First()
	} else {
		ok = it.Last()
	}
	if !ok {
		return false, nil
	}
	if err := v.Unmarshal(it.Value()); err != nil {
		return false, err
	}
	if err := o.db.Delete(it.Key(), nil); err != nil {
		return false, err
	}
	o.bytes -= v.ProtoSize()
	o.len--
	return true, nil
}

func (o *diskMap) Delete(k []byte) error {
	if err := o.db.Delete(k, nil); err != nil {
		return err
	}
	o.len--
	return nil
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

func (i *diskIterator) Value(v Value) error {
	return v.Unmarshal(i.Iterator.Value())
}

type diskReverseIterator struct {
	*diskIterator
	next func(*diskReverseIterator) bool
}

func (i *diskReverseIterator) Next() bool {
	return i.next(i)
}

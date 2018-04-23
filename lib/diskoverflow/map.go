// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package diskoverflow

import (
	"io/ioutil"
	"os"

	"github.com/syndtr/goleveldb/leveldb"
)

type Map interface {
	Common
	Add(k string, v Value)
	Get(k string) (Value, bool)
	Iter(fn func(k string, v Value) bool)
	IterAndClose(fn func(k string, v Value) bool)
	Pop(k string) (Value, bool)
}

func NewMap(location string) Map {
	m := &intMap{
		location: location,
		key:      lim.register(),
	}
	m.commonMap = &memoryMap{
		values: make(map[string]Value),
		key:    m.key,
	}
	return m
}

type commonMap interface {
	common
	add(k string, v Value)
	get(k string) (Value, bool)
	iter(fn func(k string, v Value) bool)
	pop(k string) (Value, bool)
}

type intMap struct {
	commonMap
	inactive commonMap
	location string
	key      int
	spilling bool
}

func (m *intMap) Add(k string, v Value) {
	if !m.spilling && lim.add(m.key, int64(len(k))+v.Bytes()) {
		m.inactive = m.commonMap
		m.commonMap = newDiskMap(m.location)
	}
	m.add(k, v)
}

// func (m *intMap) Bytes() int64 {
// }

func (m *intMap) Close() {
	m.close()
	if m.spilling {
		m.inactive.close()
	}
	lim.deregister(m.key)
}

func (m *intMap) Get(k string) (Value, bool) {
	if v, ok := m.get(k); ok {
		return v, true
	}
	if m.spilling {
		return m.inactive.get(k)
	}
	return nil, false
}

func (m *intMap) Iter(fn func(string, Value) bool) {
	if m.spilling {
		m.inactive.iter(fn)
	}
	m.iter(fn)
}

// Golang does not actually free memory on delete, so don't bother trying to
// release memory early (https://github.com/golang/go/issues/20135)
func (m *intMap) IterAndClose(fn func(string, Value) bool) {
	m.Iter(fn)
	m.Close()
}

func (m *intMap) Length() int {
	if !m.spilling {
		return m.length()
	}
	return m.length() + m.inactive.length()
}

func (m *intMap) Pop(k string) (Value, bool) {
	v, ok := m.pop(k)
	if !m.spilling {
		if ok {
			lim.remove(m.key, int64(len(k))+v.Bytes())
		}
		return v, ok
	}
	if ok {
		return v, true
	}
	return m.inactive.pop(k)
}

type memoryMap struct {
	values map[string]Value
	key    int
}

func (m *memoryMap) add(k string, v Value) {
	m.values[k] = v
}

func (m *memoryMap) bytes() int64 {
	return lim.bytes(m.key)
}

func (m *memoryMap) close() {
	m.values = nil
}

func (m *memoryMap) get(key string) (Value, bool) {
	v, ok := m.values[key]
	return v, ok
}

func (m *memoryMap) iter(fn func(string, Value) bool) {
	for k, v := range m.values {
		if !fn(k, v) {
			return
		}
	}
}

func (m *memoryMap) length() int {
	return len(m.values)
}

func (m *memoryMap) pop(key string) (Value, bool) {
	v, ok := m.values[key]
	if ok {
		delete(m.values, key)
	}
	return v, ok
}

type diskMap struct {
	db  *leveldb.DB
	dir string
	len int
}

func newDiskMap(location string) *diskMap {
	// Use a temporary database directory.
	tmp, err := ioutil.TempDir(location, "overflow-")
	if err != nil {
		panic("creating temporary directory: " + err.Error())
	}
	db, err := leveldb.OpenFile(tmp, levelDBOpts)
	if err != nil {
		panic("creating temporary database: " + err.Error())
	}
	return &diskMap{
		db:  db,
		dir: tmp,
	}
}

func (m *diskMap) add(k string, v Value) {
	m.addBytes([]byte(k), v)
	m.len++
}

func (m *diskMap) addBytes(k []byte, v Value) {
	if err := m.db.Put(k, v.ToByte(), nil); err != nil {
		panic("writing to temporary database: " + err.Error())
	}
}

func (m *diskMap) close() {
	m.db.Close()
	os.RemoveAll(m.dir)
}

func (m *diskMap) get(k string) (Value, bool) {
	data, err := m.db.Get([]byte(k), nil)
	if err != nil {
		return nil, false
	}
	var v Value
	v.FromByte(data)
	return v, true
}

func (m *diskMap) iter(fn func(string, Value) bool) {
	it := m.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		var v Value
		v.FromByte(it.Value())
		if !fn(string(it.Key()), v) {
			return
		}
	}
}

func (m *diskMap) length() int {
	return m.len
}

func (m *diskMap) pop(k string) (Value, bool) {
	v, ok := m.get(k)
	if ok {
		m.db.Delete([]byte(k), nil)
	}
	return v, ok
}

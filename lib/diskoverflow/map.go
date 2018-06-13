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
	"github.com/syndtr/goleveldb/leveldb/opt"
)

type Map struct {
	commonMap
	inactive commonMap
	location string
	key      int
	spilling bool
}

type commonMap interface {
	common
	add(k string, v Value)
	get(k string, v Value) (Value, bool)
	iter(fn func(k string, v Value) bool, closing bool, v Value) bool
	pop(k string, v Value) (Value, bool)
}

func NewMap(location string) *Map {
	m := &Map{
		location: location,
		key:      lim.register(),
	}
	m.commonMap = &memoryMap{
		values: make(map[string]Value),
		key:    m.key,
	}
	return m
}

func (m *Map) Add(k string, v Value) {
	if !m.spilling && !lim.add(m.key, int64(len(k))+v.Size()) {
		m.inactive = m.commonMap
		m.commonMap = newDiskMap(m.location)
		m.spilling = true
	}
	m.add(k, v)
}

func (m *Map) Close() {
	m.close()
	if m.spilling {
		m.inactive.close()
	}
	lim.deregister(m.key)
}

func (m *Map) Get(k string, v Value) (Value, bool) {
	if v, ok := m.get(k, v); ok {
		return v, true
	}
	if m.spilling {
		return m.inactive.get(k, v)
	}
	return nil, false
}

func (m *Map) Iter(fn func(string, Value) bool, v Value) {
	m.iterImpl(fn, false, v)
}

func (m *Map) IterAndClose(fn func(string, Value) bool, v Value) {
	m.iterImpl(fn, true, v)
	m.Close()
}

func (m *Map) iterImpl(fn func(string, Value) bool, closing bool, v Value) {
	if m.spilling {
		if !m.inactive.iter(fn, closing, v) {
			return
		}
	}
	m.iter(fn, closing, v)
}

func (m *Map) Length() int {
	if !m.spilling {
		return m.length()
	}
	return m.length() + m.inactive.length()
}

func (m *Map) Pop(k string, v Value) (Value, bool) {
	v, ok := m.pop(k, v)
	if !m.spilling {
		if ok {
			lim.remove(m.key, int64(len(k))+v.Size())
		}
		return v, ok
	}
	if ok {
		return v, true
	}
	return m.inactive.pop(k, v)
}

func (m *Map) String() string {
	return fmt.Sprintf("Map/%d", m.key)
}

type memoryMap struct {
	values       map[string]Value
	key          int
	deletedBytes int64
}

func (m *memoryMap) add(k string, v Value) {
	m.values[k] = v
}

func (m *memoryMap) close() {
	m.values = nil
}

func (m *memoryMap) get(key string, v Value) (Value, bool) {
	v, ok := m.values[key]
	return v, ok
}

func (m *memoryMap) iter(fn func(string, Value) bool, closing bool, _ Value) bool {
	orig := lim.size(m.key)
	for k, v := range m.values {
		if !fn(k, v) {
			return false
		}
		if closing && orig > 2*minCompactionSize {
			m.pop(k, v)
		}
	}
	return true
}

func (m *memoryMap) length() int {
	return len(m.values)
}

func (m *memoryMap) pop(key string, v Value) (Value, bool) {
	var ok bool
	v, ok = m.values[key]
	if !ok {
		return nil, false
	}
	delete(m.values, key)
	m.deletedBytes += v.Size()
	if m.deletedBytes > minCompactionSize && m.deletedBytes/lim.size(m.key) > 0 {
		newVals := make(map[string]Value, len(m.values))
		for key, val := range m.values {
			newVals[key] = val
		}
		lim.remove(m.key, m.deletedBytes)
		m.deletedBytes = 0
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

func (m *diskMap) add(k string, v Value) {
	m.addBytes([]byte(k), v)
}

func (m *diskMap) addBytes(k []byte, v Value) {
	if err := m.db.Put(k, v.Marshal(), nil); err != nil {
		panic("writing to temporary database: " + err.Error())
	}
	m.len++
}

func (m *diskMap) close() {
	m.db.Close()
	os.RemoveAll(m.dir)
}

func (m *diskMap) get(k string, v Value) (Value, bool) {
	data, err := m.db.Get([]byte(k), nil)
	if err != nil {
		return nil, false
	}
	return v.Unmarshal(data), true
}

func (m *diskMap) iter(fn func(string, Value) bool, closing bool, v Value) bool {
	it := m.db.NewIterator(nil, nil)
	defer func() {
		it.Release()
		if closing {
			m.close()
		}
	}()
	for it.Next() {
		v = v.Unmarshal(it.Value())
		if !fn(string(it.Key()), v) {
			return false
		}
	}
	return true
}

func (m *diskMap) length() int {
	return m.len
}

func (m *diskMap) pop(k string, v Value) (Value, bool) {
	v, ok := m.get(k, v)
	if ok {
		m.db.Delete([]byte(k), nil)
		m.len--
	}
	return v, ok
}

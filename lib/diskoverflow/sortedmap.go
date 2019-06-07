// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package diskoverflow

import (
	"fmt"
	"sort"
)

type SortedMap interface {
	Common
	Set(k []byte, v Value) error
	Get(k []byte, v Value) (bool, error)
	Pop(k []byte, v Value) (bool, error)
	PopFirst(v Value) (bool, error)
	PopLast(v Value) (bool, error)
	Delete(k []byte) error
	NewIterator() MapIterator
	NewReverseIterator() MapIterator
}

type sortedMap struct {
	base
	commonSortedMap
}

type commonSortedMap interface {
	veryCommonMap
	PopFirst(v Value) (bool, error)
	PopLast(v Value) (bool, error)
	newIterator(reverse bool) MapIterator
}

// NewSortedMap returns an implementation of Map, spilling to disk at location.
func NewSortedMap(location string) SortedMap {
	o := &sortedMap{base: newBase(location)}
	o.commonSortedMap = &memSortedMap{
		memMap: memMap{values: make(map[string]Value)},
	}
	return o
}

func (o *sortedMap) Set(k []byte, v Value) error {
	if o.startSpilling(o.Bytes() + v.ProtoSize()) {
		d, err := v.Marshal()
		if err != nil {
			return err
		}
		newMap, err := newDiskMap(o.location)
		if err != nil {
			return err
		}
		it := o.newIterator(false)
		for it.Next() {
			v.Reset()
			if err := it.Value(v); err != nil {
				return err
			}
			if err := newMap.set(it.Key(), v); err != nil {
				return err
			}
		}
		it.Release()
		o.commonSortedMap.Close()
		o.commonSortedMap = newMap
		o.spilling = true
		v.Reset()
		if err := v.Unmarshal(d); err != nil {
			return err
		}
	}
	return o.set(k, v)
}

func (o *sortedMap) String() string {
	return fmt.Sprintf("SortedMap@%p", o)
}

// Close is just here to catch deferred calls to Close, such that the correct
// method is called in case spilling happened.
func (o *sortedMap) Close() {
	o.commonSortedMap.Close()
}

func (o *sortedMap) NewIterator() MapIterator {
	return o.newIterator(false)
}

func (o *sortedMap) NewReverseIterator() MapIterator {
	return o.newIterator(true)
}

type memSortedMap struct {
	memMap
	needsSorting bool
	keys         []string
}

func (o *memSortedMap) set(k []byte, v Value) error {
	s := string(k)
	if _, ok := o.values[s]; !ok {
		o.needsSorting = true
		o.keys = append(o.keys, s)
	}
	return o.memMap.set(k, v)
}

func (o *memSortedMap) PopFirst(v Value) (bool, error) {
	if o.Items() == 0 {
		return false, nil
	}
	if o.needsSorting {
		sort.Strings(o.keys)
		o.needsSorting = false
	}
	for _, ok := o.values[o.keys[0]]; !ok; _, ok = o.values[o.keys[0]] {
		o.keys = o.keys[1:]
	}
	return o.Pop([]byte(o.keys[0]), v)
}

func (o *memSortedMap) PopLast(v Value) (bool, error) {
	if o.Items() == 0 {
		return false, nil
	}
	if o.needsSorting {
		sort.Strings(o.keys)
		o.needsSorting = false
	}
	for _, ok := o.values[o.keys[len(o.keys)-1]]; !ok; _, ok = o.values[o.keys[len(o.keys)-1]] {
		o.keys = o.keys[:len(o.keys)-1]
	}
	return o.Pop([]byte(o.keys[len(o.keys)-1]), v)
}

type memSortedMapIterator struct {
	*posIterator
	lastKey string
	keys    []string
	values  map[string]Value
}

func (o *memSortedMap) newIterator(reverse bool) MapIterator {
	if o.needsSorting {
		sort.Strings(o.keys)
		o.needsSorting = false
	}
	return &memSortedMapIterator{
		posIterator: newPosIterator(len(o.keys), reverse),
		keys:        o.keys,
		values:      o.values,
	}
}

func (si *memSortedMapIterator) Next() bool {
	if !si.posIterator.Next() {
		return false
	}
	// If items were removed from the map, their keys remained.
	for si.offset < si.len {
		key := si.keys[si.pos()]
		if key != si.lastKey {
			if _, ok := si.values[key]; ok {
				si.lastKey = key
				return true
			}
		}
		si.offset++
	}
	return false
}

func (si *memSortedMapIterator) Value(v Value) error {
	if si.offset >= 0 && si.offset < si.len {
		copyValue(v, si.values[si.keys[si.pos()]])
	}
	return nil
}

func (si *memSortedMapIterator) Key() []byte {
	return []byte(si.keys[si.pos()])
}

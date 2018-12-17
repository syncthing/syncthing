// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package iterator

import (
	"github.com/syndtr/goleveldb/leveldb/comparer"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type dir int

const (
	dirReleased dir = iota - 1
	dirSOI
	dirEOI
	dirBackward
	dirForward
)

type mergedIterator struct {
	cmp    comparer.Comparer
	iters  []Iterator
	strict bool

	keys     [][]byte
	index    int
	dir      dir
	err      error
	errf     func(err error)
	releaser util.Releaser
}

func assertKey(key []byte) []byte {
	if key == nil {
		panic("leveldb/iterator: nil key")
	}
	return key
}

func (i *mergedIterator) iterErr(iter Iterator) bool {
	if err := iter.Error(); err != nil {
		if i.errf != nil {
			i.errf(err)
		}
		if i.strict || !errors.IsCorrupted(err) {
			i.err = err
			return true
		}
	}
	return false
}

func (i *mergedIterator) Valid() bool {
	return i.err == nil && i.dir > dirEOI
}

func (i *mergedIterator) First() bool {
	if i.err != nil {
		return false
	} else if i.dir == dirReleased {
		i.err = ErrIterReleased
		return false
	}

	for x, iter := range i.iters {
		switch {
		case iter.First():
			i.keys[x] = assertKey(iter.Key())
		case i.iterErr(iter):
			return false
		default:
			i.keys[x] = nil
		}
	}
	i.dir = dirSOI
	return i.next()
}

func (i *mergedIterator) Last() bool {
	if i.err != nil {
		return false
	} else if i.dir == dirReleased {
		i.err = ErrIterReleased
		return false
	}

	for x, iter := range i.iters {
		switch {
		case iter.Last():
			i.keys[x] = assertKey(iter.Key())
		case i.iterErr(iter):
			return false
		default:
			i.keys[x] = nil
		}
	}
	i.dir = dirEOI
	return i.prev()
}

func (i *mergedIterator) Seek(key []byte) bool {
	if i.err != nil {
		return false
	} else if i.dir == dirReleased {
		i.err = ErrIterReleased
		return false
	}

	for x, iter := range i.iters {
		switch {
		case iter.Seek(key):
			i.keys[x] = assertKey(iter.Key())
		case i.iterErr(iter):
			return false
		default:
			i.keys[x] = nil
		}
	}
	i.dir = dirSOI
	return i.next()
}

func (i *mergedIterator) next() bool {
	var key []byte
	if i.dir == dirForward {
		key = i.keys[i.index]
	}
	for x, tkey := range i.keys {
		if tkey != nil && (key == nil || i.cmp.Compare(tkey, key) < 0) {
			key = tkey
			i.index = x
		}
	}
	if key == nil {
		i.dir = dirEOI
		return false
	}
	i.dir = dirForward
	return true
}

func (i *mergedIterator) Next() bool {
	if i.dir == dirEOI || i.err != nil {
		return false
	} else if i.dir == dirReleased {
		i.err = ErrIterReleased
		return false
	}

	switch i.dir {
	case dirSOI:
		return i.First()
	case dirBackward:
		key := append([]byte{}, i.keys[i.index]...)
		if !i.Seek(key) {
			return false
		}
		return i.Next()
	}

	x := i.index
	iter := i.iters[x]
	switch {
	case iter.Next():
		i.keys[x] = assertKey(iter.Key())
	case i.iterErr(iter):
		return false
	default:
		i.keys[x] = nil
	}
	return i.next()
}

func (i *mergedIterator) prev() bool {
	var key []byte
	if i.dir == dirBackward {
		key = i.keys[i.index]
	}
	for x, tkey := range i.keys {
		if tkey != nil && (key == nil || i.cmp.Compare(tkey, key) > 0) {
			key = tkey
			i.index = x
		}
	}
	if key == nil {
		i.dir = dirSOI
		return false
	}
	i.dir = dirBackward
	return true
}

func (i *mergedIterator) Prev() bool {
	if i.dir == dirSOI || i.err != nil {
		return false
	} else if i.dir == dirReleased {
		i.err = ErrIterReleased
		return false
	}

	switch i.dir {
	case dirEOI:
		return i.Last()
	case dirForward:
		key := append([]byte{}, i.keys[i.index]...)
		for x, iter := range i.iters {
			if x == i.index {
				continue
			}
			seek := iter.Seek(key)
			switch {
			case seek && iter.Prev(), !seek && iter.Last():
				i.keys[x] = assertKey(iter.Key())
			case i.iterErr(iter):
				return false
			default:
				i.keys[x] = nil
			}
		}
	}

	x := i.index
	iter := i.iters[x]
	switch {
	case iter.Prev():
		i.keys[x] = assertKey(iter.Key())
	case i.iterErr(iter):
		return false
	default:
		i.keys[x] = nil
	}
	return i.prev()
}

func (i *mergedIterator) Key() []byte {
	if i.err != nil || i.dir <= dirEOI {
		return nil
	}
	return i.keys[i.index]
}

func (i *mergedIterator) Value() []byte {
	if i.err != nil || i.dir <= dirEOI {
		return nil
	}
	return i.iters[i.index].Value()
}

func (i *mergedIterator) Release() {
	if i.dir != dirReleased {
		i.dir = dirReleased
		for _, iter := range i.iters {
			iter.Release()
		}
		i.iters = nil
		i.keys = nil
		if i.releaser != nil {
			i.releaser.Release()
			i.releaser = nil
		}
	}
}

func (i *mergedIterator) SetReleaser(releaser util.Releaser) {
	if i.dir == dirReleased {
		panic(util.ErrReleased)
	}
	if i.releaser != nil && releaser != nil {
		panic(util.ErrHasReleaser)
	}
	i.releaser = releaser
}

func (i *mergedIterator) Error() error {
	return i.err
}

func (i *mergedIterator) SetErrorCallback(f func(err error)) {
	i.errf = f
}

// NewMergedIterator returns an iterator that merges its input. Walking the
// resultant iterator will return all key/value pairs of all input iterators
// in strictly increasing key order, as defined by cmp.
// The input's key ranges may overlap, but there are assumed to be no duplicate
// keys: if iters[i] contains a key k then iters[j] will not contain that key k.
// None of the iters may be nil.
//
// If strict is true the any 'corruption errors' (i.e errors.IsCorrupted(err) == true)
// won't be ignored and will halt 'merged iterator', otherwise the iterator will
// continue to the next 'input iterator'.
func NewMergedIterator(iters []Iterator, cmp comparer.Comparer, strict bool) Iterator {
	return &mergedIterator{
		iters:  iters,
		cmp:    cmp,
		strict: strict,
		keys:   make([][]byte, len(iters)),
	}
}

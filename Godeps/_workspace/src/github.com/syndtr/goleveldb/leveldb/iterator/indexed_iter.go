// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package iterator

import (
	"github.com/syndtr/goleveldb/leveldb/util"
)

// IteratorIndexer is the interface that wraps CommonIterator and basic Get
// method. IteratorIndexer provides index for indexed iterator.
type IteratorIndexer interface {
	CommonIterator

	// Get returns a new data iterator for the current position, or nil if
	// done.
	Get() Iterator
}

type indexedIterator struct {
	util.BasicReleaser
	index     IteratorIndexer
	strict    bool
	strictGet bool

	data Iterator
	err  error
	errf func(err error)
}

func (i *indexedIterator) setData() {
	if i.data != nil {
		i.data.Release()
	}
	i.data = i.index.Get()
	if i.strictGet {
		if err := i.data.Error(); err != nil {
			i.err = err
		}
	}
}

func (i *indexedIterator) clearData() {
	if i.data != nil {
		i.data.Release()
	}
	i.data = nil
}

func (i *indexedIterator) dataErr() bool {
	if i.errf != nil {
		if err := i.data.Error(); err != nil {
			i.errf(err)
		}
	}
	if i.strict {
		if err := i.data.Error(); err != nil {
			i.err = err
			return true
		}
	}
	return false
}

func (i *indexedIterator) Valid() bool {
	return i.data != nil && i.data.Valid()
}

func (i *indexedIterator) First() bool {
	if i.err != nil {
		return false
	}

	if !i.index.First() {
		i.clearData()
		return false
	}
	i.setData()
	return i.Next()
}

func (i *indexedIterator) Last() bool {
	if i.err != nil {
		return false
	}

	if !i.index.Last() {
		i.clearData()
		return false
	}
	i.setData()
	if !i.data.Last() {
		if i.dataErr() {
			return false
		}
		i.clearData()
		return i.Prev()
	}
	return true
}

func (i *indexedIterator) Seek(key []byte) bool {
	if i.err != nil {
		return false
	}

	if !i.index.Seek(key) {
		i.clearData()
		return false
	}
	i.setData()
	if !i.data.Seek(key) {
		if i.dataErr() {
			return false
		}
		i.clearData()
		return i.Next()
	}
	return true
}

func (i *indexedIterator) Next() bool {
	if i.err != nil {
		return false
	}

	switch {
	case i.data != nil && !i.data.Next():
		if i.dataErr() {
			return false
		}
		i.clearData()
		fallthrough
	case i.data == nil:
		if !i.index.Next() {
			return false
		}
		i.setData()
		return i.Next()
	}
	return true
}

func (i *indexedIterator) Prev() bool {
	if i.err != nil {
		return false
	}

	switch {
	case i.data != nil && !i.data.Prev():
		if i.dataErr() {
			return false
		}
		i.clearData()
		fallthrough
	case i.data == nil:
		if !i.index.Prev() {
			return false
		}
		i.setData()
		if !i.data.Last() {
			if i.dataErr() {
				return false
			}
			i.clearData()
			return i.Prev()
		}
	}
	return true
}

func (i *indexedIterator) Key() []byte {
	if i.data == nil {
		return nil
	}
	return i.data.Key()
}

func (i *indexedIterator) Value() []byte {
	if i.data == nil {
		return nil
	}
	return i.data.Value()
}

func (i *indexedIterator) Release() {
	i.clearData()
	i.index.Release()
	i.BasicReleaser.Release()
}

func (i *indexedIterator) Error() error {
	if i.err != nil {
		return i.err
	}
	if err := i.index.Error(); err != nil {
		return err
	}
	return nil
}

func (i *indexedIterator) SetErrorCallback(f func(err error)) {
	i.errf = f
}

// NewIndexedIterator returns an indexed iterator. An index is iterator
// that returns another iterator, a data iterator. A data iterator is the
// iterator that contains actual key/value pairs.
//
// If strict is true then error yield by data iterator will halt the indexed
// iterator, on contrary if strict is false then the indexed iterator will
// ignore those error and move on to the next index. If strictGet is true and
// index.Get() yield an 'error iterator' then the indexed iterator will be halted.
// An 'error iterator' is iterator which its Error() method always return non-nil
// even before any 'seeks method' is called.
func NewIndexedIterator(index IteratorIndexer, strict, strictGet bool) Iterator {
	return &indexedIterator{index: index, strict: strict, strictGet: strictGet}
}

// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"errors"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var (
	errInvalidInternalKey = errors.New("leveldb: Iterator: invalid internal key")
)

type memdbReleaser struct {
	once sync.Once
	m    *memDB
}

func (mr *memdbReleaser) Release() {
	mr.once.Do(func() {
		mr.m.decref()
	})
}

func (db *DB) newRawIterator(auxm *memDB, auxt tFiles, slice *util.Range, ro *opt.ReadOptions) iterator.Iterator {
	strict := opt.GetStrict(db.s.o.Options, ro, opt.StrictReader)
	em, fm := db.getMems()
	v := db.s.version()

	tableIts := v.getIterators(slice, ro)
	n := len(tableIts) + len(auxt) + 3
	its := make([]iterator.Iterator, 0, n)

	if auxm != nil {
		ami := auxm.NewIterator(slice)
		ami.SetReleaser(&memdbReleaser{m: auxm})
		its = append(its, ami)
	}
	for _, t := range auxt {
		its = append(its, v.s.tops.newIterator(t, slice, ro))
	}

	emi := em.NewIterator(slice)
	emi.SetReleaser(&memdbReleaser{m: em})
	its = append(its, emi)
	if fm != nil {
		fmi := fm.NewIterator(slice)
		fmi.SetReleaser(&memdbReleaser{m: fm})
		its = append(its, fmi)
	}
	its = append(its, tableIts...)
	mi := iterator.NewMergedIterator(its, db.s.icmp, strict)
	mi.SetReleaser(&versionReleaser{v: v})
	return mi
}

func (db *DB) newIterator(auxm *memDB, auxt tFiles, seq uint64, slice *util.Range, ro *opt.ReadOptions) *dbIter {
	var islice *util.Range
	if slice != nil {
		islice = &util.Range{}
		if slice.Start != nil {
			islice.Start = makeInternalKey(nil, slice.Start, keyMaxSeq, keyTypeSeek)
		}
		if slice.Limit != nil {
			islice.Limit = makeInternalKey(nil, slice.Limit, keyMaxSeq, keyTypeSeek)
		}
	}
	rawIter := db.newRawIterator(auxm, auxt, islice, ro)
	iter := &dbIter{
		db:     db,
		icmp:   db.s.icmp,
		iter:   rawIter,
		seq:    seq,
		strict: opt.GetStrict(db.s.o.Options, ro, opt.StrictReader),
		key:    make([]byte, 0),
		value:  make([]byte, 0),
	}
	atomic.AddInt32(&db.aliveIters, 1)
	runtime.SetFinalizer(iter, (*dbIter).Release)
	return iter
}

func (db *DB) iterSamplingRate() int {
	return rand.Intn(2 * db.s.o.GetIteratorSamplingRate())
}

type dir int

const (
	dirReleased dir = iota - 1
	dirSOI
	dirEOI
	dirBackward
	dirForward
)

// dbIter represent an interator states over a database session.
type dbIter struct {
	db     *DB
	icmp   *iComparer
	iter   iterator.Iterator
	seq    uint64
	strict bool

	smaplingGap int
	dir         dir
	key         []byte
	value       []byte
	err         error
	releaser    util.Releaser
}

func (i *dbIter) sampleSeek() {
	ikey := i.iter.Key()
	i.smaplingGap -= len(ikey) + len(i.iter.Value())
	for i.smaplingGap < 0 {
		i.smaplingGap += i.db.iterSamplingRate()
		i.db.sampleSeek(ikey)
	}
}

func (i *dbIter) setErr(err error) {
	i.err = err
	i.key = nil
	i.value = nil
}

func (i *dbIter) iterErr() {
	if err := i.iter.Error(); err != nil {
		i.setErr(err)
	}
}

func (i *dbIter) Valid() bool {
	return i.err == nil && i.dir > dirEOI
}

func (i *dbIter) First() bool {
	if i.err != nil {
		return false
	} else if i.dir == dirReleased {
		i.err = ErrIterReleased
		return false
	}

	if i.iter.First() {
		i.dir = dirSOI
		return i.next()
	}
	i.dir = dirEOI
	i.iterErr()
	return false
}

func (i *dbIter) Last() bool {
	if i.err != nil {
		return false
	} else if i.dir == dirReleased {
		i.err = ErrIterReleased
		return false
	}

	if i.iter.Last() {
		return i.prev()
	}
	i.dir = dirSOI
	i.iterErr()
	return false
}

func (i *dbIter) Seek(key []byte) bool {
	if i.err != nil {
		return false
	} else if i.dir == dirReleased {
		i.err = ErrIterReleased
		return false
	}

	ikey := makeInternalKey(nil, key, i.seq, keyTypeSeek)
	if i.iter.Seek(ikey) {
		i.dir = dirSOI
		return i.next()
	}
	i.dir = dirEOI
	i.iterErr()
	return false
}

func (i *dbIter) next() bool {
	for {
		if ukey, seq, kt, kerr := parseInternalKey(i.iter.Key()); kerr == nil {
			i.sampleSeek()
			if seq <= i.seq {
				switch kt {
				case keyTypeDel:
					// Skip deleted key.
					i.key = append(i.key[:0], ukey...)
					i.dir = dirForward
				case keyTypeVal:
					if i.dir == dirSOI || i.icmp.uCompare(ukey, i.key) > 0 {
						i.key = append(i.key[:0], ukey...)
						i.value = append(i.value[:0], i.iter.Value()...)
						i.dir = dirForward
						return true
					}
				}
			}
		} else if i.strict {
			i.setErr(kerr)
			break
		}
		if !i.iter.Next() {
			i.dir = dirEOI
			i.iterErr()
			break
		}
	}
	return false
}

func (i *dbIter) Next() bool {
	if i.dir == dirEOI || i.err != nil {
		return false
	} else if i.dir == dirReleased {
		i.err = ErrIterReleased
		return false
	}

	if !i.iter.Next() || (i.dir == dirBackward && !i.iter.Next()) {
		i.dir = dirEOI
		i.iterErr()
		return false
	}
	return i.next()
}

func (i *dbIter) prev() bool {
	i.dir = dirBackward
	del := true
	if i.iter.Valid() {
		for {
			if ukey, seq, kt, kerr := parseInternalKey(i.iter.Key()); kerr == nil {
				i.sampleSeek()
				if seq <= i.seq {
					if !del && i.icmp.uCompare(ukey, i.key) < 0 {
						return true
					}
					del = (kt == keyTypeDel)
					if !del {
						i.key = append(i.key[:0], ukey...)
						i.value = append(i.value[:0], i.iter.Value()...)
					}
				}
			} else if i.strict {
				i.setErr(kerr)
				return false
			}
			if !i.iter.Prev() {
				break
			}
		}
	}
	if del {
		i.dir = dirSOI
		i.iterErr()
		return false
	}
	return true
}

func (i *dbIter) Prev() bool {
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
		for i.iter.Prev() {
			if ukey, _, _, kerr := parseInternalKey(i.iter.Key()); kerr == nil {
				i.sampleSeek()
				if i.icmp.uCompare(ukey, i.key) < 0 {
					goto cont
				}
			} else if i.strict {
				i.setErr(kerr)
				return false
			}
		}
		i.dir = dirSOI
		i.iterErr()
		return false
	}

cont:
	return i.prev()
}

func (i *dbIter) Key() []byte {
	if i.err != nil || i.dir <= dirEOI {
		return nil
	}
	return i.key
}

func (i *dbIter) Value() []byte {
	if i.err != nil || i.dir <= dirEOI {
		return nil
	}
	return i.value
}

func (i *dbIter) Release() {
	if i.dir != dirReleased {
		// Clear the finalizer.
		runtime.SetFinalizer(i, nil)

		if i.releaser != nil {
			i.releaser.Release()
			i.releaser = nil
		}

		i.dir = dirReleased
		i.key = nil
		i.value = nil
		i.iter.Release()
		i.iter = nil
		atomic.AddInt32(&i.db.aliveIters, -1)
		i.db = nil
	}
}

func (i *dbIter) SetReleaser(releaser util.Releaser) {
	if i.dir == dirReleased {
		panic(util.ErrReleased)
	}
	if i.releaser != nil && releaser != nil {
		panic(util.ErrHasReleaser)
	}
	i.releaser = releaser
}

func (i *dbIter) Error() error {
	return i.err
}

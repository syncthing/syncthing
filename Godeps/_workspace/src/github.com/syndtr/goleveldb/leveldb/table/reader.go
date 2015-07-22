// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package table

import (
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/golang/snappy"

	"github.com/syndtr/goleveldb/leveldb/cache"
	"github.com/syndtr/goleveldb/leveldb/comparer"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var (
	ErrNotFound       = errors.ErrNotFound
	ErrReaderReleased = errors.New("leveldb/table: reader released")
	ErrIterReleased   = errors.New("leveldb/table: iterator released")
)

type ErrCorrupted struct {
	Pos    int64
	Size   int64
	Kind   string
	Reason string
}

func (e *ErrCorrupted) Error() string {
	return fmt.Sprintf("leveldb/table: corruption on %s (pos=%d): %s", e.Kind, e.Pos, e.Reason)
}

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

type block struct {
	bpool          *util.BufferPool
	bh             blockHandle
	data           []byte
	restartsLen    int
	restartsOffset int
}

func (b *block) seek(cmp comparer.Comparer, rstart, rlimit int, key []byte) (index, offset int, err error) {
	index = sort.Search(b.restartsLen-rstart-(b.restartsLen-rlimit), func(i int) bool {
		offset := int(binary.LittleEndian.Uint32(b.data[b.restartsOffset+4*(rstart+i):]))
		offset += 1                                 // shared always zero, since this is a restart point
		v1, n1 := binary.Uvarint(b.data[offset:])   // key length
		_, n2 := binary.Uvarint(b.data[offset+n1:]) // value length
		m := offset + n1 + n2
		return cmp.Compare(b.data[m:m+int(v1)], key) > 0
	}) + rstart - 1
	if index < rstart {
		// The smallest key is greater-than key sought.
		index = rstart
	}
	offset = int(binary.LittleEndian.Uint32(b.data[b.restartsOffset+4*index:]))
	return
}

func (b *block) restartIndex(rstart, rlimit, offset int) int {
	return sort.Search(b.restartsLen-rstart-(b.restartsLen-rlimit), func(i int) bool {
		return int(binary.LittleEndian.Uint32(b.data[b.restartsOffset+4*(rstart+i):])) > offset
	}) + rstart - 1
}

func (b *block) restartOffset(index int) int {
	return int(binary.LittleEndian.Uint32(b.data[b.restartsOffset+4*index:]))
}

func (b *block) entry(offset int) (key, value []byte, nShared, n int, err error) {
	if offset >= b.restartsOffset {
		if offset != b.restartsOffset {
			err = &ErrCorrupted{Reason: "entries offset not aligned"}
		}
		return
	}
	v0, n0 := binary.Uvarint(b.data[offset:])       // Shared prefix length
	v1, n1 := binary.Uvarint(b.data[offset+n0:])    // Key length
	v2, n2 := binary.Uvarint(b.data[offset+n0+n1:]) // Value length
	m := n0 + n1 + n2
	n = m + int(v1) + int(v2)
	if n0 <= 0 || n1 <= 0 || n2 <= 0 || offset+n > b.restartsOffset {
		err = &ErrCorrupted{Reason: "entries corrupted"}
		return
	}
	key = b.data[offset+m : offset+m+int(v1)]
	value = b.data[offset+m+int(v1) : offset+n]
	nShared = int(v0)
	return
}

func (b *block) Release() {
	b.bpool.Put(b.data)
	b.bpool = nil
	b.data = nil
}

type dir int

const (
	dirReleased dir = iota - 1
	dirSOI
	dirEOI
	dirBackward
	dirForward
)

type blockIter struct {
	tr            *Reader
	block         *block
	blockReleaser util.Releaser
	releaser      util.Releaser
	key, value    []byte
	offset        int
	// Previous offset, only filled by Next.
	prevOffset   int
	prevNode     []int
	prevKeys     []byte
	restartIndex int
	// Iterator direction.
	dir dir
	// Restart index slice range.
	riStart int
	riLimit int
	// Offset slice range.
	offsetStart     int
	offsetRealStart int
	offsetLimit     int
	// Error.
	err error
}

func (i *blockIter) sErr(err error) {
	i.err = err
	i.key = nil
	i.value = nil
	i.prevNode = nil
	i.prevKeys = nil
}

func (i *blockIter) reset() {
	if i.dir == dirBackward {
		i.prevNode = i.prevNode[:0]
		i.prevKeys = i.prevKeys[:0]
	}
	i.restartIndex = i.riStart
	i.offset = i.offsetStart
	i.dir = dirSOI
	i.key = i.key[:0]
	i.value = nil
}

func (i *blockIter) isFirst() bool {
	switch i.dir {
	case dirForward:
		return i.prevOffset == i.offsetRealStart
	case dirBackward:
		return len(i.prevNode) == 1 && i.restartIndex == i.riStart
	}
	return false
}

func (i *blockIter) isLast() bool {
	switch i.dir {
	case dirForward, dirBackward:
		return i.offset == i.offsetLimit
	}
	return false
}

func (i *blockIter) First() bool {
	if i.err != nil {
		return false
	} else if i.dir == dirReleased {
		i.err = ErrIterReleased
		return false
	}

	if i.dir == dirBackward {
		i.prevNode = i.prevNode[:0]
		i.prevKeys = i.prevKeys[:0]
	}
	i.dir = dirSOI
	return i.Next()
}

func (i *blockIter) Last() bool {
	if i.err != nil {
		return false
	} else if i.dir == dirReleased {
		i.err = ErrIterReleased
		return false
	}

	if i.dir == dirBackward {
		i.prevNode = i.prevNode[:0]
		i.prevKeys = i.prevKeys[:0]
	}
	i.dir = dirEOI
	return i.Prev()
}

func (i *blockIter) Seek(key []byte) bool {
	if i.err != nil {
		return false
	} else if i.dir == dirReleased {
		i.err = ErrIterReleased
		return false
	}

	ri, offset, err := i.block.seek(i.tr.cmp, i.riStart, i.riLimit, key)
	if err != nil {
		i.sErr(err)
		return false
	}
	i.restartIndex = ri
	i.offset = max(i.offsetStart, offset)
	if i.dir == dirSOI || i.dir == dirEOI {
		i.dir = dirForward
	}
	for i.Next() {
		if i.tr.cmp.Compare(i.key, key) >= 0 {
			return true
		}
	}
	return false
}

func (i *blockIter) Next() bool {
	if i.dir == dirEOI || i.err != nil {
		return false
	} else if i.dir == dirReleased {
		i.err = ErrIterReleased
		return false
	}

	if i.dir == dirSOI {
		i.restartIndex = i.riStart
		i.offset = i.offsetStart
	} else if i.dir == dirBackward {
		i.prevNode = i.prevNode[:0]
		i.prevKeys = i.prevKeys[:0]
	}
	for i.offset < i.offsetRealStart {
		key, value, nShared, n, err := i.block.entry(i.offset)
		if err != nil {
			i.sErr(i.tr.fixErrCorruptedBH(i.block.bh, err))
			return false
		}
		if n == 0 {
			i.dir = dirEOI
			return false
		}
		i.key = append(i.key[:nShared], key...)
		i.value = value
		i.offset += n
	}
	if i.offset >= i.offsetLimit {
		i.dir = dirEOI
		if i.offset != i.offsetLimit {
			i.sErr(i.tr.newErrCorruptedBH(i.block.bh, "entries offset not aligned"))
		}
		return false
	}
	key, value, nShared, n, err := i.block.entry(i.offset)
	if err != nil {
		i.sErr(i.tr.fixErrCorruptedBH(i.block.bh, err))
		return false
	}
	if n == 0 {
		i.dir = dirEOI
		return false
	}
	i.key = append(i.key[:nShared], key...)
	i.value = value
	i.prevOffset = i.offset
	i.offset += n
	i.dir = dirForward
	return true
}

func (i *blockIter) Prev() bool {
	if i.dir == dirSOI || i.err != nil {
		return false
	} else if i.dir == dirReleased {
		i.err = ErrIterReleased
		return false
	}

	var ri int
	if i.dir == dirForward {
		// Change direction.
		i.offset = i.prevOffset
		if i.offset == i.offsetRealStart {
			i.dir = dirSOI
			return false
		}
		ri = i.block.restartIndex(i.restartIndex, i.riLimit, i.offset)
		i.dir = dirBackward
	} else if i.dir == dirEOI {
		// At the end of iterator.
		i.restartIndex = i.riLimit
		i.offset = i.offsetLimit
		if i.offset == i.offsetRealStart {
			i.dir = dirSOI
			return false
		}
		ri = i.riLimit - 1
		i.dir = dirBackward
	} else if len(i.prevNode) == 1 {
		// This is the end of a restart range.
		i.offset = i.prevNode[0]
		i.prevNode = i.prevNode[:0]
		if i.restartIndex == i.riStart {
			i.dir = dirSOI
			return false
		}
		i.restartIndex--
		ri = i.restartIndex
	} else {
		// In the middle of restart range, get from cache.
		n := len(i.prevNode) - 3
		node := i.prevNode[n:]
		i.prevNode = i.prevNode[:n]
		// Get the key.
		ko := node[0]
		i.key = append(i.key[:0], i.prevKeys[ko:]...)
		i.prevKeys = i.prevKeys[:ko]
		// Get the value.
		vo := node[1]
		vl := vo + node[2]
		i.value = i.block.data[vo:vl]
		i.offset = vl
		return true
	}
	// Build entries cache.
	i.key = i.key[:0]
	i.value = nil
	offset := i.block.restartOffset(ri)
	if offset == i.offset {
		ri -= 1
		if ri < 0 {
			i.dir = dirSOI
			return false
		}
		offset = i.block.restartOffset(ri)
	}
	i.prevNode = append(i.prevNode, offset)
	for {
		key, value, nShared, n, err := i.block.entry(offset)
		if err != nil {
			i.sErr(i.tr.fixErrCorruptedBH(i.block.bh, err))
			return false
		}
		if offset >= i.offsetRealStart {
			if i.value != nil {
				// Appends 3 variables:
				// 1. Previous keys offset
				// 2. Value offset in the data block
				// 3. Value length
				i.prevNode = append(i.prevNode, len(i.prevKeys), offset-len(i.value), len(i.value))
				i.prevKeys = append(i.prevKeys, i.key...)
			}
			i.value = value
		}
		i.key = append(i.key[:nShared], key...)
		offset += n
		// Stop if target offset reached.
		if offset >= i.offset {
			if offset != i.offset {
				i.sErr(i.tr.newErrCorruptedBH(i.block.bh, "entries offset not aligned"))
				return false
			}

			break
		}
	}
	i.restartIndex = ri
	i.offset = offset
	return true
}

func (i *blockIter) Key() []byte {
	if i.err != nil || i.dir <= dirEOI {
		return nil
	}
	return i.key
}

func (i *blockIter) Value() []byte {
	if i.err != nil || i.dir <= dirEOI {
		return nil
	}
	return i.value
}

func (i *blockIter) Release() {
	if i.dir != dirReleased {
		i.tr = nil
		i.block = nil
		i.prevNode = nil
		i.prevKeys = nil
		i.key = nil
		i.value = nil
		i.dir = dirReleased
		if i.blockReleaser != nil {
			i.blockReleaser.Release()
			i.blockReleaser = nil
		}
		if i.releaser != nil {
			i.releaser.Release()
			i.releaser = nil
		}
	}
}

func (i *blockIter) SetReleaser(releaser util.Releaser) {
	if i.dir == dirReleased {
		panic(util.ErrReleased)
	}
	if i.releaser != nil && releaser != nil {
		panic(util.ErrHasReleaser)
	}
	i.releaser = releaser
}

func (i *blockIter) Valid() bool {
	return i.err == nil && (i.dir == dirBackward || i.dir == dirForward)
}

func (i *blockIter) Error() error {
	return i.err
}

type filterBlock struct {
	bpool      *util.BufferPool
	data       []byte
	oOffset    int
	baseLg     uint
	filtersNum int
}

func (b *filterBlock) contains(filter filter.Filter, offset uint64, key []byte) bool {
	i := int(offset >> b.baseLg)
	if i < b.filtersNum {
		o := b.data[b.oOffset+i*4:]
		n := int(binary.LittleEndian.Uint32(o))
		m := int(binary.LittleEndian.Uint32(o[4:]))
		if n < m && m <= b.oOffset {
			return filter.Contains(b.data[n:m], key)
		} else if n == m {
			return false
		}
	}
	return true
}

func (b *filterBlock) Release() {
	b.bpool.Put(b.data)
	b.bpool = nil
	b.data = nil
}

type indexIter struct {
	*blockIter
	tr    *Reader
	slice *util.Range
	// Options
	fillCache bool
}

func (i *indexIter) Get() iterator.Iterator {
	value := i.Value()
	if value == nil {
		return nil
	}
	dataBH, n := decodeBlockHandle(value)
	if n == 0 {
		return iterator.NewEmptyIterator(i.tr.newErrCorruptedBH(i.tr.indexBH, "bad data block handle"))
	}

	var slice *util.Range
	if i.slice != nil && (i.blockIter.isFirst() || i.blockIter.isLast()) {
		slice = i.slice
	}
	return i.tr.getDataIterErr(dataBH, slice, i.tr.verifyChecksum, i.fillCache)
}

// Reader is a table reader.
type Reader struct {
	mu     sync.RWMutex
	fi     *storage.FileInfo
	reader io.ReaderAt
	cache  *cache.CacheGetter
	err    error
	bpool  *util.BufferPool
	// Options
	o              *opt.Options
	cmp            comparer.Comparer
	filter         filter.Filter
	verifyChecksum bool

	dataEnd                   int64
	metaBH, indexBH, filterBH blockHandle
	indexBlock                *block
	filterBlock               *filterBlock
}

func (r *Reader) blockKind(bh blockHandle) string {
	switch bh.offset {
	case r.metaBH.offset:
		return "meta-block"
	case r.indexBH.offset:
		return "index-block"
	case r.filterBH.offset:
		if r.filterBH.length > 0 {
			return "filter-block"
		}
	}
	return "data-block"
}

func (r *Reader) newErrCorrupted(pos, size int64, kind, reason string) error {
	return &errors.ErrCorrupted{File: r.fi, Err: &ErrCorrupted{Pos: pos, Size: size, Kind: kind, Reason: reason}}
}

func (r *Reader) newErrCorruptedBH(bh blockHandle, reason string) error {
	return r.newErrCorrupted(int64(bh.offset), int64(bh.length), r.blockKind(bh), reason)
}

func (r *Reader) fixErrCorruptedBH(bh blockHandle, err error) error {
	if cerr, ok := err.(*ErrCorrupted); ok {
		cerr.Pos = int64(bh.offset)
		cerr.Size = int64(bh.length)
		cerr.Kind = r.blockKind(bh)
		return &errors.ErrCorrupted{File: r.fi, Err: cerr}
	}
	return err
}

func (r *Reader) readRawBlock(bh blockHandle, verifyChecksum bool) ([]byte, error) {
	data := r.bpool.Get(int(bh.length + blockTrailerLen))
	if _, err := r.reader.ReadAt(data, int64(bh.offset)); err != nil && err != io.EOF {
		return nil, err
	}

	if verifyChecksum {
		n := bh.length + 1
		checksum0 := binary.LittleEndian.Uint32(data[n:])
		checksum1 := util.NewCRC(data[:n]).Value()
		if checksum0 != checksum1 {
			r.bpool.Put(data)
			return nil, r.newErrCorruptedBH(bh, fmt.Sprintf("checksum mismatch, want=%#x got=%#x", checksum0, checksum1))
		}
	}

	switch data[bh.length] {
	case blockTypeNoCompression:
		data = data[:bh.length]
	case blockTypeSnappyCompression:
		decLen, err := snappy.DecodedLen(data[:bh.length])
		if err != nil {
			return nil, r.newErrCorruptedBH(bh, err.Error())
		}
		decData := r.bpool.Get(decLen)
		decData, err = snappy.Decode(decData, data[:bh.length])
		r.bpool.Put(data)
		if err != nil {
			r.bpool.Put(decData)
			return nil, r.newErrCorruptedBH(bh, err.Error())
		}
		data = decData
	default:
		r.bpool.Put(data)
		return nil, r.newErrCorruptedBH(bh, fmt.Sprintf("unknown compression type %#x", data[bh.length]))
	}
	return data, nil
}

func (r *Reader) readBlock(bh blockHandle, verifyChecksum bool) (*block, error) {
	data, err := r.readRawBlock(bh, verifyChecksum)
	if err != nil {
		return nil, err
	}
	restartsLen := int(binary.LittleEndian.Uint32(data[len(data)-4:]))
	b := &block{
		bpool:          r.bpool,
		bh:             bh,
		data:           data,
		restartsLen:    restartsLen,
		restartsOffset: len(data) - (restartsLen+1)*4,
	}
	return b, nil
}

func (r *Reader) readBlockCached(bh blockHandle, verifyChecksum, fillCache bool) (*block, util.Releaser, error) {
	if r.cache != nil {
		var (
			err error
			ch  *cache.Handle
		)
		if fillCache {
			ch = r.cache.Get(bh.offset, func() (size int, value cache.Value) {
				var b *block
				b, err = r.readBlock(bh, verifyChecksum)
				if err != nil {
					return 0, nil
				}
				return cap(b.data), b
			})
		} else {
			ch = r.cache.Get(bh.offset, nil)
		}
		if ch != nil {
			b, ok := ch.Value().(*block)
			if !ok {
				ch.Release()
				return nil, nil, errors.New("leveldb/table: inconsistent block type")
			}
			return b, ch, err
		} else if err != nil {
			return nil, nil, err
		}
	}

	b, err := r.readBlock(bh, verifyChecksum)
	return b, b, err
}

func (r *Reader) readFilterBlock(bh blockHandle) (*filterBlock, error) {
	data, err := r.readRawBlock(bh, true)
	if err != nil {
		return nil, err
	}
	n := len(data)
	if n < 5 {
		return nil, r.newErrCorruptedBH(bh, "too short")
	}
	m := n - 5
	oOffset := int(binary.LittleEndian.Uint32(data[m:]))
	if oOffset > m {
		return nil, r.newErrCorruptedBH(bh, "invalid data-offsets offset")
	}
	b := &filterBlock{
		bpool:      r.bpool,
		data:       data,
		oOffset:    oOffset,
		baseLg:     uint(data[n-1]),
		filtersNum: (m - oOffset) / 4,
	}
	return b, nil
}

func (r *Reader) readFilterBlockCached(bh blockHandle, fillCache bool) (*filterBlock, util.Releaser, error) {
	if r.cache != nil {
		var (
			err error
			ch  *cache.Handle
		)
		if fillCache {
			ch = r.cache.Get(bh.offset, func() (size int, value cache.Value) {
				var b *filterBlock
				b, err = r.readFilterBlock(bh)
				if err != nil {
					return 0, nil
				}
				return cap(b.data), b
			})
		} else {
			ch = r.cache.Get(bh.offset, nil)
		}
		if ch != nil {
			b, ok := ch.Value().(*filterBlock)
			if !ok {
				ch.Release()
				return nil, nil, errors.New("leveldb/table: inconsistent block type")
			}
			return b, ch, err
		} else if err != nil {
			return nil, nil, err
		}
	}

	b, err := r.readFilterBlock(bh)
	return b, b, err
}

func (r *Reader) getIndexBlock(fillCache bool) (b *block, rel util.Releaser, err error) {
	if r.indexBlock == nil {
		return r.readBlockCached(r.indexBH, true, fillCache)
	}
	return r.indexBlock, util.NoopReleaser{}, nil
}

func (r *Reader) getFilterBlock(fillCache bool) (*filterBlock, util.Releaser, error) {
	if r.filterBlock == nil {
		return r.readFilterBlockCached(r.filterBH, fillCache)
	}
	return r.filterBlock, util.NoopReleaser{}, nil
}

func (r *Reader) newBlockIter(b *block, bReleaser util.Releaser, slice *util.Range, inclLimit bool) *blockIter {
	bi := &blockIter{
		tr:            r,
		block:         b,
		blockReleaser: bReleaser,
		// Valid key should never be nil.
		key:             make([]byte, 0),
		dir:             dirSOI,
		riStart:         0,
		riLimit:         b.restartsLen,
		offsetStart:     0,
		offsetRealStart: 0,
		offsetLimit:     b.restartsOffset,
	}
	if slice != nil {
		if slice.Start != nil {
			if bi.Seek(slice.Start) {
				bi.riStart = b.restartIndex(bi.restartIndex, b.restartsLen, bi.prevOffset)
				bi.offsetStart = b.restartOffset(bi.riStart)
				bi.offsetRealStart = bi.prevOffset
			} else {
				bi.riStart = b.restartsLen
				bi.offsetStart = b.restartsOffset
				bi.offsetRealStart = b.restartsOffset
			}
		}
		if slice.Limit != nil {
			if bi.Seek(slice.Limit) && (!inclLimit || bi.Next()) {
				bi.offsetLimit = bi.prevOffset
				bi.riLimit = bi.restartIndex + 1
			}
		}
		bi.reset()
		if bi.offsetStart > bi.offsetLimit {
			bi.sErr(errors.New("leveldb/table: invalid slice range"))
		}
	}
	return bi
}

func (r *Reader) getDataIter(dataBH blockHandle, slice *util.Range, verifyChecksum, fillCache bool) iterator.Iterator {
	b, rel, err := r.readBlockCached(dataBH, verifyChecksum, fillCache)
	if err != nil {
		return iterator.NewEmptyIterator(err)
	}
	return r.newBlockIter(b, rel, slice, false)
}

func (r *Reader) getDataIterErr(dataBH blockHandle, slice *util.Range, verifyChecksum, fillCache bool) iterator.Iterator {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.err != nil {
		return iterator.NewEmptyIterator(r.err)
	}

	return r.getDataIter(dataBH, slice, verifyChecksum, fillCache)
}

// NewIterator creates an iterator from the table.
//
// Slice allows slicing the iterator to only contains keys in the given
// range. A nil Range.Start is treated as a key before all keys in the
// table. And a nil Range.Limit is treated as a key after all keys in
// the table.
//
// The returned iterator is not goroutine-safe and should be released
// when not used.
//
// Also read Iterator documentation of the leveldb/iterator package.
func (r *Reader) NewIterator(slice *util.Range, ro *opt.ReadOptions) iterator.Iterator {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.err != nil {
		return iterator.NewEmptyIterator(r.err)
	}

	fillCache := !ro.GetDontFillCache()
	indexBlock, rel, err := r.getIndexBlock(fillCache)
	if err != nil {
		return iterator.NewEmptyIterator(err)
	}
	index := &indexIter{
		blockIter: r.newBlockIter(indexBlock, rel, slice, true),
		tr:        r,
		slice:     slice,
		fillCache: !ro.GetDontFillCache(),
	}
	return iterator.NewIndexedIterator(index, opt.GetStrict(r.o, ro, opt.StrictReader))
}

func (r *Reader) find(key []byte, filtered bool, ro *opt.ReadOptions, noValue bool) (rkey, value []byte, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.err != nil {
		err = r.err
		return
	}

	indexBlock, rel, err := r.getIndexBlock(true)
	if err != nil {
		return
	}
	defer rel.Release()

	index := r.newBlockIter(indexBlock, nil, nil, true)
	defer index.Release()
	if !index.Seek(key) {
		err = index.Error()
		if err == nil {
			err = ErrNotFound
		}
		return
	}
	dataBH, n := decodeBlockHandle(index.Value())
	if n == 0 {
		r.err = r.newErrCorruptedBH(r.indexBH, "bad data block handle")
		return
	}
	if filtered && r.filter != nil {
		filterBlock, frel, ferr := r.getFilterBlock(true)
		if ferr == nil {
			if !filterBlock.contains(r.filter, dataBH.offset, key) {
				frel.Release()
				return nil, nil, ErrNotFound
			}
			frel.Release()
		} else if !errors.IsCorrupted(ferr) {
			err = ferr
			return
		}
	}
	data := r.getDataIter(dataBH, nil, r.verifyChecksum, !ro.GetDontFillCache())
	defer data.Release()
	if !data.Seek(key) {
		err = data.Error()
		if err == nil {
			err = ErrNotFound
		}
		return
	}
	// Don't use block buffer, no need to copy the buffer.
	rkey = data.Key()
	if !noValue {
		if r.bpool == nil {
			value = data.Value()
		} else {
			// Use block buffer, and since the buffer will be recycled, the buffer
			// need to be copied.
			value = append([]byte{}, data.Value()...)
		}
	}
	return
}

// Find finds key/value pair whose key is greater than or equal to the
// given key. It returns ErrNotFound if the table doesn't contain
// such pair.
// If filtered is true then the nearest 'block' will be checked against
// 'filter data' (if present) and will immediately return ErrNotFound if
// 'filter data' indicates that such pair doesn't exist.
//
// The caller may modify the contents of the returned slice as it is its
// own copy.
// It is safe to modify the contents of the argument after Find returns.
func (r *Reader) Find(key []byte, filtered bool, ro *opt.ReadOptions) (rkey, value []byte, err error) {
	return r.find(key, filtered, ro, false)
}

// Find finds key that is greater than or equal to the given key.
// It returns ErrNotFound if the table doesn't contain such key.
// If filtered is true then the nearest 'block' will be checked against
// 'filter data' (if present) and will immediately return ErrNotFound if
// 'filter data' indicates that such key doesn't exist.
//
// The caller may modify the contents of the returned slice as it is its
// own copy.
// It is safe to modify the contents of the argument after Find returns.
func (r *Reader) FindKey(key []byte, filtered bool, ro *opt.ReadOptions) (rkey []byte, err error) {
	rkey, _, err = r.find(key, filtered, ro, true)
	return
}

// Get gets the value for the given key. It returns errors.ErrNotFound
// if the table does not contain the key.
//
// The caller may modify the contents of the returned slice as it is its
// own copy.
// It is safe to modify the contents of the argument after Find returns.
func (r *Reader) Get(key []byte, ro *opt.ReadOptions) (value []byte, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.err != nil {
		err = r.err
		return
	}

	rkey, value, err := r.find(key, false, ro, false)
	if err == nil && r.cmp.Compare(rkey, key) != 0 {
		value = nil
		err = ErrNotFound
	}
	return
}

// OffsetOf returns approximate offset for the given key.
//
// It is safe to modify the contents of the argument after Get returns.
func (r *Reader) OffsetOf(key []byte) (offset int64, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.err != nil {
		err = r.err
		return
	}

	indexBlock, rel, err := r.readBlockCached(r.indexBH, true, true)
	if err != nil {
		return
	}
	defer rel.Release()

	index := r.newBlockIter(indexBlock, nil, nil, true)
	defer index.Release()
	if index.Seek(key) {
		dataBH, n := decodeBlockHandle(index.Value())
		if n == 0 {
			r.err = r.newErrCorruptedBH(r.indexBH, "bad data block handle")
			return
		}
		offset = int64(dataBH.offset)
		return
	}
	err = index.Error()
	if err == nil {
		offset = r.dataEnd
	}
	return
}

// Release implements util.Releaser.
// It also close the file if it is an io.Closer.
func (r *Reader) Release() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if closer, ok := r.reader.(io.Closer); ok {
		closer.Close()
	}
	if r.indexBlock != nil {
		r.indexBlock.Release()
		r.indexBlock = nil
	}
	if r.filterBlock != nil {
		r.filterBlock.Release()
		r.filterBlock = nil
	}
	r.reader = nil
	r.cache = nil
	r.bpool = nil
	r.err = ErrReaderReleased
}

// NewReader creates a new initialized table reader for the file.
// The fi, cache and bpool is optional and can be nil.
//
// The returned table reader instance is goroutine-safe.
func NewReader(f io.ReaderAt, size int64, fi *storage.FileInfo, cache *cache.CacheGetter, bpool *util.BufferPool, o *opt.Options) (*Reader, error) {
	if f == nil {
		return nil, errors.New("leveldb/table: nil file")
	}

	r := &Reader{
		fi:             fi,
		reader:         f,
		cache:          cache,
		bpool:          bpool,
		o:              o,
		cmp:            o.GetComparer(),
		verifyChecksum: o.GetStrict(opt.StrictBlockChecksum),
	}

	if size < footerLen {
		r.err = r.newErrCorrupted(0, size, "table", "too small")
		return r, nil
	}

	footerPos := size - footerLen
	var footer [footerLen]byte
	if _, err := r.reader.ReadAt(footer[:], footerPos); err != nil && err != io.EOF {
		return nil, err
	}
	if string(footer[footerLen-len(magic):footerLen]) != magic {
		r.err = r.newErrCorrupted(footerPos, footerLen, "table-footer", "bad magic number")
		return r, nil
	}

	var n int
	// Decode the metaindex block handle.
	r.metaBH, n = decodeBlockHandle(footer[:])
	if n == 0 {
		r.err = r.newErrCorrupted(footerPos, footerLen, "table-footer", "bad metaindex block handle")
		return r, nil
	}

	// Decode the index block handle.
	r.indexBH, n = decodeBlockHandle(footer[n:])
	if n == 0 {
		r.err = r.newErrCorrupted(footerPos, footerLen, "table-footer", "bad index block handle")
		return r, nil
	}

	// Read metaindex block.
	metaBlock, err := r.readBlock(r.metaBH, true)
	if err != nil {
		if errors.IsCorrupted(err) {
			r.err = err
			return r, nil
		} else {
			return nil, err
		}
	}

	// Set data end.
	r.dataEnd = int64(r.metaBH.offset)

	// Read metaindex.
	metaIter := r.newBlockIter(metaBlock, nil, nil, true)
	for metaIter.Next() {
		key := string(metaIter.Key())
		if !strings.HasPrefix(key, "filter.") {
			continue
		}
		fn := key[7:]
		if f0 := o.GetFilter(); f0 != nil && f0.Name() == fn {
			r.filter = f0
		} else {
			for _, f0 := range o.GetAltFilters() {
				if f0.Name() == fn {
					r.filter = f0
					break
				}
			}
		}
		if r.filter != nil {
			filterBH, n := decodeBlockHandle(metaIter.Value())
			if n == 0 {
				continue
			}
			r.filterBH = filterBH
			// Update data end.
			r.dataEnd = int64(filterBH.offset)
			break
		}
	}
	metaIter.Release()
	metaBlock.Release()

	// Cache index and filter block locally, since we don't have global cache.
	if cache == nil {
		r.indexBlock, err = r.readBlock(r.indexBH, true)
		if err != nil {
			if errors.IsCorrupted(err) {
				r.err = err
				return r, nil
			} else {
				return nil, err
			}
		}
		if r.filter != nil {
			r.filterBlock, err = r.readFilterBlock(r.filterBH)
			if err != nil {
				if !errors.IsCorrupted(err) {
					return nil, err
				}

				// Don't use filter then.
				r.filter = nil
			}
		}
	}

	return r, nil
}

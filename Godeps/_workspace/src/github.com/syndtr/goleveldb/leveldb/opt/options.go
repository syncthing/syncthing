// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package opt provides sets of options used by LevelDB.
package opt

import (
	"github.com/syndtr/goleveldb/leveldb/cache"
	"github.com/syndtr/goleveldb/leveldb/comparer"
	"github.com/syndtr/goleveldb/leveldb/filter"
)

const (
	KiB = 1024
	MiB = KiB * 1024
	GiB = MiB * 1024
)

const (
	DefaultBlockCacheSize       = 8 * MiB
	DefaultBlockRestartInterval = 16
	DefaultBlockSize            = 4 * KiB
	DefaultCompressionType      = SnappyCompression
	DefaultMaxOpenFiles         = 1000
	DefaultWriteBuffer          = 4 * MiB
)

type noCache struct{}

func (noCache) SetCapacity(capacity int)               {}
func (noCache) GetNamespace(id uint64) cache.Namespace { return nil }
func (noCache) Purge(fin cache.PurgeFin)               {}
func (noCache) Zap(closed bool)                        {}

var NoCache cache.Cache = noCache{}

// Compression is the per-block compression algorithm to use.
type Compression uint

func (c Compression) String() string {
	switch c {
	case DefaultCompression:
		return "default"
	case NoCompression:
		return "none"
	case SnappyCompression:
		return "snappy"
	}
	return "invalid"
}

const (
	DefaultCompression Compression = iota
	NoCompression
	SnappyCompression
	nCompression
)

// Strict is the DB strict level.
type Strict uint

const (
	// If present then a corrupted or invalid chunk or block in manifest
	// journal will cause an error istead of being dropped.
	StrictManifest Strict = 1 << iota

	// If present then a corrupted or invalid chunk or block in journal
	// will cause an error istead of being dropped.
	StrictJournal

	// If present then journal chunk checksum will be verified.
	StrictJournalChecksum

	// If present then an invalid key/value pair will cause an error
	// instead of being skipped.
	StrictIterator

	// If present then 'sorted table' block checksum will be verified.
	StrictBlockChecksum

	// StrictAll enables all strict flags.
	StrictAll = StrictManifest | StrictJournal | StrictJournalChecksum | StrictIterator | StrictBlockChecksum

	// DefaultStrict is the default strict flags. Specify any strict flags
	// will override default strict flags as whole (i.e. not OR'ed).
	DefaultStrict = StrictJournalChecksum | StrictBlockChecksum

	// NoStrict disables all strict flags. Override default strict flags.
	NoStrict = ^StrictAll
)

// Options holds the optional parameters for the DB at large.
type Options struct {
	// AltFilters defines one or more 'alternative filters'.
	// 'alternative filters' will be used during reads if a filter block
	// does not match with the 'effective filter'.
	//
	// The default value is nil
	AltFilters []filter.Filter

	// BlockCache provides per-block caching for LevelDB. Specify NoCache to
	// disable block caching.
	//
	// By default LevelDB will create LRU-cache with capacity of 8MiB.
	BlockCache cache.Cache

	// BlockRestartInterval is the number of keys between restart points for
	// delta encoding of keys.
	//
	// The default value is 16.
	BlockRestartInterval int

	// BlockSize is the minimum uncompressed size in bytes of each 'sorted table'
	// block.
	//
	// The default value is 4KiB.
	BlockSize int

	// Comparer defines a total ordering over the space of []byte keys: a 'less
	// than' relationship. The same comparison algorithm must be used for reads
	// and writes over the lifetime of the DB.
	//
	// The default value uses the same ordering as bytes.Compare.
	Comparer comparer.Comparer

	// Compression defines the per-block compression to use.
	//
	// The default value (DefaultCompression) uses snappy compression.
	Compression Compression

	// ErrorIfExist defines whether an error should returned if the DB already
	// exist.
	//
	// The default value is false.
	ErrorIfExist bool

	// ErrorIfMissing defines whether an error should returned if the DB is
	// missing. If false then the database will be created if missing, otherwise
	// an error will be returned.
	//
	// The default value is false.
	ErrorIfMissing bool

	// Filter defines an 'effective filter' to use. An 'effective filter'
	// if defined will be used to generate per-table filter block.
	// The filter name will be stored on disk.
	// During reads LevelDB will try to find matching filter from
	// 'effective filter' and 'alternative filters'.
	//
	// Filter can be changed after a DB has been created. It is recommended
	// to put old filter to the 'alternative filters' to mitigate lack of
	// filter during transition period.
	//
	// A filter is used to reduce disk reads when looking for a specific key.
	//
	// The default value is nil.
	Filter filter.Filter

	// MaxOpenFiles defines maximum number of open files to kept around
	// (cached). This is not an hard limit, actual open files may exceed
	// the defined value.
	//
	// The default value is 1000.
	MaxOpenFiles int

	// Strict defines the DB strict level.
	Strict Strict

	// WriteBuffer defines maximum size of a 'memdb' before flushed to
	// 'sorted table'. 'memdb' is an in-memory DB backed by an on-disk
	// unsorted journal.
	//
	// LevelDB may held up to two 'memdb' at the same time.
	//
	// The default value is 4MiB.
	WriteBuffer int
}

func (o *Options) GetAltFilters() []filter.Filter {
	if o == nil {
		return nil
	}
	return o.AltFilters
}

func (o *Options) GetBlockCache() cache.Cache {
	if o == nil {
		return nil
	}
	return o.BlockCache
}

func (o *Options) GetBlockRestartInterval() int {
	if o == nil || o.BlockRestartInterval <= 0 {
		return DefaultBlockRestartInterval
	}
	return o.BlockRestartInterval
}

func (o *Options) GetBlockSize() int {
	if o == nil || o.BlockSize <= 0 {
		return DefaultBlockSize
	}
	return o.BlockSize
}

func (o *Options) GetComparer() comparer.Comparer {
	if o == nil || o.Comparer == nil {
		return comparer.DefaultComparer
	}
	return o.Comparer
}

func (o *Options) GetCompression() Compression {
	if o == nil || o.Compression <= DefaultCompression || o.Compression >= nCompression {
		return DefaultCompressionType
	}
	return o.Compression
}

func (o *Options) GetErrorIfExist() bool {
	if o == nil {
		return false
	}
	return o.ErrorIfExist
}

func (o *Options) GetErrorIfMissing() bool {
	if o == nil {
		return false
	}
	return o.ErrorIfMissing
}

func (o *Options) GetFilter() filter.Filter {
	if o == nil {
		return nil
	}
	return o.Filter
}

func (o *Options) GetMaxOpenFiles() int {
	if o == nil || o.MaxOpenFiles <= 0 {
		return DefaultMaxOpenFiles
	}
	return o.MaxOpenFiles
}

func (o *Options) GetStrict(strict Strict) bool {
	if o == nil || o.Strict == 0 {
		return DefaultStrict&strict != 0
	}
	return o.Strict&strict != 0
}

func (o *Options) GetWriteBuffer() int {
	if o == nil || o.WriteBuffer <= 0 {
		return DefaultWriteBuffer
	}
	return o.WriteBuffer
}

// ReadOptions holds the optional parameters for 'read operation'. The
// 'read operation' includes Get, Find and NewIterator.
type ReadOptions struct {
	// DontFillCache defines whether block reads for this 'read operation'
	// should be cached. If false then the block will be cached. This does
	// not affects already cached block.
	//
	// The default value is false.
	DontFillCache bool

	// Strict overrides global DB strict level. Only StrictIterator and
	// StrictBlockChecksum that does have effects here.
	Strict Strict
}

func (ro *ReadOptions) GetDontFillCache() bool {
	if ro == nil {
		return false
	}
	return ro.DontFillCache
}

func (ro *ReadOptions) GetStrict(strict Strict) bool {
	if ro == nil {
		return false
	}
	return ro.Strict&strict != 0
}

// WriteOptions holds the optional parameters for 'write operation'. The
// 'write operation' includes Write, Put and Delete.
type WriteOptions struct {
	// Sync is whether to sync underlying writes from the OS buffer cache
	// through to actual disk, if applicable. Setting Sync can result in
	// slower writes.
	//
	// If false, and the machine crashes, then some recent writes may be lost.
	// Note that if it is just the process that crashes (and the machine does
	// not) then no writes will be lost.
	//
	// In other words, Sync being false has the same semantics as a write
	// system call. Sync being true means write followed by fsync.
	//
	// The default value is false.
	Sync bool
}

func (wo *WriteOptions) GetSync() bool {
	if wo == nil {
		return false
	}
	return wo.Sync
}

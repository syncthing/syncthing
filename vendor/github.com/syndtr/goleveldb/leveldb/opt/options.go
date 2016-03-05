// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package opt provides sets of options used by LevelDB.
package opt

import (
	"math"

	"github.com/syndtr/goleveldb/leveldb/cache"
	"github.com/syndtr/goleveldb/leveldb/comparer"
	"github.com/syndtr/goleveldb/leveldb/filter"
)

const (
	KiB = 1024
	MiB = KiB * 1024
	GiB = MiB * 1024
)

var (
	DefaultBlockCacher                   = LRUCacher
	DefaultBlockCacheCapacity            = 8 * MiB
	DefaultBlockRestartInterval          = 16
	DefaultBlockSize                     = 4 * KiB
	DefaultCompactionExpandLimitFactor   = 25
	DefaultCompactionGPOverlapsFactor    = 10
	DefaultCompactionL0Trigger           = 4
	DefaultCompactionSourceLimitFactor   = 1
	DefaultCompactionTableSize           = 2 * MiB
	DefaultCompactionTableSizeMultiplier = 1.0
	DefaultCompactionTotalSize           = 10 * MiB
	DefaultCompactionTotalSizeMultiplier = 10.0
	DefaultCompressionType               = SnappyCompression
	DefaultIteratorSamplingRate          = 1 * MiB
	DefaultOpenFilesCacher               = LRUCacher
	DefaultOpenFilesCacheCapacity        = 500
	DefaultWriteBuffer                   = 4 * MiB
	DefaultWriteL0PauseTrigger           = 12
	DefaultWriteL0SlowdownTrigger        = 8
)

// Cacher is a caching algorithm.
type Cacher interface {
	New(capacity int) cache.Cacher
}

type CacherFunc struct {
	NewFunc func(capacity int) cache.Cacher
}

func (f *CacherFunc) New(capacity int) cache.Cacher {
	if f.NewFunc != nil {
		return f.NewFunc(capacity)
	}
	return nil
}

func noCacher(int) cache.Cacher { return nil }

var (
	// LRUCacher is the LRU-cache algorithm.
	LRUCacher = &CacherFunc{cache.NewLRU}

	// NoCacher is the value to disable caching algorithm.
	NoCacher = &CacherFunc{}
)

// Compression is the 'sorted table' block compression algorithm to use.
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

// Strict is the DB 'strict level'.
type Strict uint

const (
	// If present then a corrupted or invalid chunk or block in manifest
	// journal will cause an error instead of being dropped.
	// This will prevent database with corrupted manifest to be opened.
	StrictManifest Strict = 1 << iota

	// If present then journal chunk checksum will be verified.
	StrictJournalChecksum

	// If present then a corrupted or invalid chunk or block in journal
	// will cause an error instead of being dropped.
	// This will prevent database with corrupted journal to be opened.
	StrictJournal

	// If present then 'sorted table' block checksum will be verified.
	// This has effect on both 'read operation' and compaction.
	StrictBlockChecksum

	// If present then a corrupted 'sorted table' will fails compaction.
	// The database will enter read-only mode.
	StrictCompaction

	// If present then a corrupted 'sorted table' will halts 'read operation'.
	StrictReader

	// If present then leveldb.Recover will drop corrupted 'sorted table'.
	StrictRecovery

	// This only applicable for ReadOptions, if present then this ReadOptions
	// 'strict level' will override global ones.
	StrictOverride

	// StrictAll enables all strict flags.
	StrictAll = StrictManifest | StrictJournalChecksum | StrictJournal | StrictBlockChecksum | StrictCompaction | StrictReader | StrictRecovery

	// DefaultStrict is the default strict flags. Specify any strict flags
	// will override default strict flags as whole (i.e. not OR'ed).
	DefaultStrict = StrictJournalChecksum | StrictBlockChecksum | StrictCompaction | StrictReader

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

	// BlockCacher provides cache algorithm for LevelDB 'sorted table' block caching.
	// Specify NoCacher to disable caching algorithm.
	//
	// The default value is LRUCacher.
	BlockCacher Cacher

	// BlockCacheCapacity defines the capacity of the 'sorted table' block caching.
	// Use -1 for zero, this has same effect as specifying NoCacher to BlockCacher.
	//
	// The default value is 8MiB.
	BlockCacheCapacity int

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

	// CompactionExpandLimitFactor limits compaction size after expanded.
	// This will be multiplied by table size limit at compaction target level.
	//
	// The default value is 25.
	CompactionExpandLimitFactor int

	// CompactionGPOverlapsFactor limits overlaps in grandparent (Level + 2) that a
	// single 'sorted table' generates.
	// This will be multiplied by table size limit at grandparent level.
	//
	// The default value is 10.
	CompactionGPOverlapsFactor int

	// CompactionL0Trigger defines number of 'sorted table' at level-0 that will
	// trigger compaction.
	//
	// The default value is 4.
	CompactionL0Trigger int

	// CompactionSourceLimitFactor limits compaction source size. This doesn't apply to
	// level-0.
	// This will be multiplied by table size limit at compaction target level.
	//
	// The default value is 1.
	CompactionSourceLimitFactor int

	// CompactionTableSize limits size of 'sorted table' that compaction generates.
	// The limits for each level will be calculated as:
	//   CompactionTableSize * (CompactionTableSizeMultiplier ^ Level)
	// The multiplier for each level can also fine-tuned using CompactionTableSizeMultiplierPerLevel.
	//
	// The default value is 2MiB.
	CompactionTableSize int

	// CompactionTableSizeMultiplier defines multiplier for CompactionTableSize.
	//
	// The default value is 1.
	CompactionTableSizeMultiplier float64

	// CompactionTableSizeMultiplierPerLevel defines per-level multiplier for
	// CompactionTableSize.
	// Use zero to skip a level.
	//
	// The default value is nil.
	CompactionTableSizeMultiplierPerLevel []float64

	// CompactionTotalSize limits total size of 'sorted table' for each level.
	// The limits for each level will be calculated as:
	//   CompactionTotalSize * (CompactionTotalSizeMultiplier ^ Level)
	// The multiplier for each level can also fine-tuned using
	// CompactionTotalSizeMultiplierPerLevel.
	//
	// The default value is 10MiB.
	CompactionTotalSize int

	// CompactionTotalSizeMultiplier defines multiplier for CompactionTotalSize.
	//
	// The default value is 10.
	CompactionTotalSizeMultiplier float64

	// CompactionTotalSizeMultiplierPerLevel defines per-level multiplier for
	// CompactionTotalSize.
	// Use zero to skip a level.
	//
	// The default value is nil.
	CompactionTotalSizeMultiplierPerLevel []float64

	// Comparer defines a total ordering over the space of []byte keys: a 'less
	// than' relationship. The same comparison algorithm must be used for reads
	// and writes over the lifetime of the DB.
	//
	// The default value uses the same ordering as bytes.Compare.
	Comparer comparer.Comparer

	// Compression defines the 'sorted table' block compression to use.
	//
	// The default value (DefaultCompression) uses snappy compression.
	Compression Compression

	// DisableBufferPool allows disable use of util.BufferPool functionality.
	//
	// The default value is false.
	DisableBufferPool bool

	// DisableBlockCache allows disable use of cache.Cache functionality on
	// 'sorted table' block.
	//
	// The default value is false.
	DisableBlockCache bool

	// DisableCompactionBackoff allows disable compaction retry backoff.
	//
	// The default value is false.
	DisableCompactionBackoff bool

	// DisableLargeBatchTransaction allows disabling switch-to-transaction mode
	// on large batch write. If enable batch writes large than WriteBuffer will
	// use transaction.
	//
	// The default is false.
	DisableLargeBatchTransaction bool

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

	// IteratorSamplingRate defines approximate gap (in bytes) between read
	// sampling of an iterator. The samples will be used to determine when
	// compaction should be triggered.
	//
	// The default is 1MiB.
	IteratorSamplingRate int

	// NoSync allows completely disable fsync.
	//
	// The default is false.
	NoSync bool

	// OpenFilesCacher provides cache algorithm for open files caching.
	// Specify NoCacher to disable caching algorithm.
	//
	// The default value is LRUCacher.
	OpenFilesCacher Cacher

	// OpenFilesCacheCapacity defines the capacity of the open files caching.
	// Use -1 for zero, this has same effect as specifying NoCacher to OpenFilesCacher.
	//
	// The default value is 500.
	OpenFilesCacheCapacity int

	// If true then opens DB in read-only mode.
	//
	// The default value is false.
	ReadOnly bool

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

	// WriteL0StopTrigger defines number of 'sorted table' at level-0 that will
	// pause write.
	//
	// The default value is 12.
	WriteL0PauseTrigger int

	// WriteL0SlowdownTrigger defines number of 'sorted table' at level-0 that
	// will trigger write slowdown.
	//
	// The default value is 8.
	WriteL0SlowdownTrigger int
}

func (o *Options) GetAltFilters() []filter.Filter {
	if o == nil {
		return nil
	}
	return o.AltFilters
}

func (o *Options) GetBlockCacher() Cacher {
	if o == nil || o.BlockCacher == nil {
		return DefaultBlockCacher
	} else if o.BlockCacher == NoCacher {
		return nil
	}
	return o.BlockCacher
}

func (o *Options) GetBlockCacheCapacity() int {
	if o == nil || o.BlockCacheCapacity == 0 {
		return DefaultBlockCacheCapacity
	} else if o.BlockCacheCapacity < 0 {
		return 0
	}
	return o.BlockCacheCapacity
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

func (o *Options) GetCompactionExpandLimit(level int) int {
	factor := DefaultCompactionExpandLimitFactor
	if o != nil && o.CompactionExpandLimitFactor > 0 {
		factor = o.CompactionExpandLimitFactor
	}
	return o.GetCompactionTableSize(level+1) * factor
}

func (o *Options) GetCompactionGPOverlaps(level int) int {
	factor := DefaultCompactionGPOverlapsFactor
	if o != nil && o.CompactionGPOverlapsFactor > 0 {
		factor = o.CompactionGPOverlapsFactor
	}
	return o.GetCompactionTableSize(level+2) * factor
}

func (o *Options) GetCompactionL0Trigger() int {
	if o == nil || o.CompactionL0Trigger == 0 {
		return DefaultCompactionL0Trigger
	}
	return o.CompactionL0Trigger
}

func (o *Options) GetCompactionSourceLimit(level int) int {
	factor := DefaultCompactionSourceLimitFactor
	if o != nil && o.CompactionSourceLimitFactor > 0 {
		factor = o.CompactionSourceLimitFactor
	}
	return o.GetCompactionTableSize(level+1) * factor
}

func (o *Options) GetCompactionTableSize(level int) int {
	var (
		base = DefaultCompactionTableSize
		mult float64
	)
	if o != nil {
		if o.CompactionTableSize > 0 {
			base = o.CompactionTableSize
		}
		if level < len(o.CompactionTableSizeMultiplierPerLevel) && o.CompactionTableSizeMultiplierPerLevel[level] > 0 {
			mult = o.CompactionTableSizeMultiplierPerLevel[level]
		} else if o.CompactionTableSizeMultiplier > 0 {
			mult = math.Pow(o.CompactionTableSizeMultiplier, float64(level))
		}
	}
	if mult == 0 {
		mult = math.Pow(DefaultCompactionTableSizeMultiplier, float64(level))
	}
	return int(float64(base) * mult)
}

func (o *Options) GetCompactionTotalSize(level int) int64 {
	var (
		base = DefaultCompactionTotalSize
		mult float64
	)
	if o != nil {
		if o.CompactionTotalSize > 0 {
			base = o.CompactionTotalSize
		}
		if level < len(o.CompactionTotalSizeMultiplierPerLevel) && o.CompactionTotalSizeMultiplierPerLevel[level] > 0 {
			mult = o.CompactionTotalSizeMultiplierPerLevel[level]
		} else if o.CompactionTotalSizeMultiplier > 0 {
			mult = math.Pow(o.CompactionTotalSizeMultiplier, float64(level))
		}
	}
	if mult == 0 {
		mult = math.Pow(DefaultCompactionTotalSizeMultiplier, float64(level))
	}
	return int64(float64(base) * mult)
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

func (o *Options) GetDisableBufferPool() bool {
	if o == nil {
		return false
	}
	return o.DisableBufferPool
}

func (o *Options) GetDisableBlockCache() bool {
	if o == nil {
		return false
	}
	return o.DisableBlockCache
}

func (o *Options) GetDisableCompactionBackoff() bool {
	if o == nil {
		return false
	}
	return o.DisableCompactionBackoff
}

func (o *Options) GetDisableLargeBatchTransaction() bool {
	if o == nil {
		return false
	}
	return o.DisableLargeBatchTransaction
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

func (o *Options) GetIteratorSamplingRate() int {
	if o == nil || o.IteratorSamplingRate <= 0 {
		return DefaultIteratorSamplingRate
	}
	return o.IteratorSamplingRate
}

func (o *Options) GetNoSync() bool {
	if o == nil {
		return false
	}
	return o.NoSync
}

func (o *Options) GetOpenFilesCacher() Cacher {
	if o == nil || o.OpenFilesCacher == nil {
		return DefaultOpenFilesCacher
	}
	if o.OpenFilesCacher == NoCacher {
		return nil
	}
	return o.OpenFilesCacher
}

func (o *Options) GetOpenFilesCacheCapacity() int {
	if o == nil || o.OpenFilesCacheCapacity == 0 {
		return DefaultOpenFilesCacheCapacity
	} else if o.OpenFilesCacheCapacity < 0 {
		return 0
	}
	return o.OpenFilesCacheCapacity
}

func (o *Options) GetReadOnly() bool {
	if o == nil {
		return false
	}
	return o.ReadOnly
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

func (o *Options) GetWriteL0PauseTrigger() int {
	if o == nil || o.WriteL0PauseTrigger == 0 {
		return DefaultWriteL0PauseTrigger
	}
	return o.WriteL0PauseTrigger
}

func (o *Options) GetWriteL0SlowdownTrigger() int {
	if o == nil || o.WriteL0SlowdownTrigger == 0 {
		return DefaultWriteL0SlowdownTrigger
	}
	return o.WriteL0SlowdownTrigger
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

	// Strict will be OR'ed with global DB 'strict level' unless StrictOverride
	// is present. Currently only StrictReader that has effect here.
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

func GetStrict(o *Options, ro *ReadOptions, strict Strict) bool {
	if ro.GetStrict(StrictOverride) {
		return ro.GetStrict(strict)
	} else {
		return o.GetStrict(strict) || ro.GetStrict(strict)
	}
}

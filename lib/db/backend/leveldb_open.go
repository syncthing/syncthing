// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package backend

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const (
	dbMaxOpenFiles = 100

	// A large database is > 200 MiB. It's a mostly arbitrary value, but
	// it's also the case that each file is 2 MiB by default and when we
	// have dbMaxOpenFiles of them we will need to start thrashing fd:s.
	// Switching to large database settings causes larger files to be used
	// when compacting, reducing the number.
	dbLargeThreshold = dbMaxOpenFiles * (2 << MiB)

	KiB = 10
	MiB = 20
)

// OpenLevelDB attempts to open the database at the given location, and runs
// recovery on it if opening fails. Worst case, if recovery is not possible,
// the database is erased and created from scratch.
func OpenLevelDB(location string, tuning Tuning) (Backend, error) {
	opts := optsFor(location, tuning)
	ldb, err := open(location, opts)
	if err != nil {
		return nil, err
	}
	return newLeveldbBackend(ldb, location), nil
}

// OpenLevelDBAuto is OpenLevelDB with TuningAuto tuning.
func OpenLevelDBAuto(location string) (Backend, error) {
	return OpenLevelDB(location, TuningAuto)
}

// OpenLevelDBRO attempts to open the database at the given location, read
// only.
func OpenLevelDBRO(location string) (Backend, error) {
	opts := &opt.Options{
		OpenFilesCacheCapacity: dbMaxOpenFiles,
		ReadOnly:               true,
	}
	ldb, err := open(location, opts)
	if err != nil {
		return nil, err
	}
	return newLeveldbBackend(ldb, location), nil
}

// OpenMemory returns a new Backend referencing an in-memory database.
func OpenLevelDBMemory() Backend {
	ldb, _ := leveldb.Open(storage.NewMemStorage(), nil)
	return newLeveldbBackend(ldb, "")
}

// optsFor returns the database options to use when opening a database with
// the given location and tuning. Settings can be overridden by debug
// environment variables.
func optsFor(location string, tuning Tuning) *opt.Options {
	large := false
	switch tuning {
	case TuningLarge:
		large = true
	case TuningAuto:
		large = dbIsLarge(location)
	}

	var (
		// Set defaults used for small databases.
		defaultBlockCacheCapacity            = 0 // 0 means let leveldb use default
		defaultBlockSize                     = 0
		defaultCompactionTableSize           = 0
		defaultCompactionTableSizeMultiplier = 0
		defaultWriteBuffer                   = 16 << MiB                      // increased from leveldb default of 4 MiB
		defaultCompactionL0Trigger           = opt.DefaultCompactionL0Trigger // explicit because we use it as base for other stuff
	)

	if large {
		// Change the parameters for better throughput at the price of some
		// RAM and larger files. This results in larger batches of writes
		// and compaction at a lower frequency.
		l.Infoln("Using large-database tuning")

		defaultBlockCacheCapacity = 64 << MiB
		defaultBlockSize = 64 << KiB
		defaultCompactionTableSize = 16 << MiB
		defaultCompactionTableSizeMultiplier = 20 // 2.0 after division by ten
		defaultWriteBuffer = 64 << MiB
		defaultCompactionL0Trigger = 8 // number of l0 files
	}

	opts := &opt.Options{
		BlockCacheCapacity:            debugEnvValue("BlockCacheCapacity", defaultBlockCacheCapacity),
		BlockCacheEvictRemoved:        debugEnvValue("BlockCacheEvictRemoved", 0) != 0,
		BlockRestartInterval:          debugEnvValue("BlockRestartInterval", 0),
		BlockSize:                     debugEnvValue("BlockSize", defaultBlockSize),
		CompactionExpandLimitFactor:   debugEnvValue("CompactionExpandLimitFactor", 0),
		CompactionGPOverlapsFactor:    debugEnvValue("CompactionGPOverlapsFactor", 0),
		CompactionL0Trigger:           debugEnvValue("CompactionL0Trigger", defaultCompactionL0Trigger),
		CompactionSourceLimitFactor:   debugEnvValue("CompactionSourceLimitFactor", 0),
		CompactionTableSize:           debugEnvValue("CompactionTableSize", defaultCompactionTableSize),
		CompactionTableSizeMultiplier: float64(debugEnvValue("CompactionTableSizeMultiplier", defaultCompactionTableSizeMultiplier)) / 10.0,
		CompactionTotalSize:           debugEnvValue("CompactionTotalSize", 0),
		CompactionTotalSizeMultiplier: float64(debugEnvValue("CompactionTotalSizeMultiplier", 0)) / 10.0,
		DisableBufferPool:             debugEnvValue("DisableBufferPool", 0) != 0,
		DisableBlockCache:             debugEnvValue("DisableBlockCache", 0) != 0,
		DisableCompactionBackoff:      debugEnvValue("DisableCompactionBackoff", 0) != 0,
		DisableLargeBatchTransaction:  debugEnvValue("DisableLargeBatchTransaction", 0) != 0,
		NoSync:                        debugEnvValue("NoSync", 0) != 0,
		NoWriteMerge:                  debugEnvValue("NoWriteMerge", 0) != 0,
		OpenFilesCacheCapacity:        debugEnvValue("OpenFilesCacheCapacity", dbMaxOpenFiles),
		WriteBuffer:                   debugEnvValue("WriteBuffer", defaultWriteBuffer),
		// The write slowdown and pause can be overridden, but even if they
		// are not and the compaction trigger is overridden we need to
		// adjust so that we don't pause writes for L0 compaction before we
		// even *start* L0 compaction...
		WriteL0SlowdownTrigger: debugEnvValue("WriteL0SlowdownTrigger", 2*debugEnvValue("CompactionL0Trigger", defaultCompactionL0Trigger)),
		WriteL0PauseTrigger:    debugEnvValue("WriteL0SlowdownTrigger", 3*debugEnvValue("CompactionL0Trigger", defaultCompactionL0Trigger)),
	}

	return opts
}

func open(location string, opts *opt.Options) (*leveldb.DB, error) {
	db, err := leveldb.OpenFile(location, opts)
	if leveldbIsCorrupted(err) {
		db, err = leveldb.RecoverFile(location, opts)
	}
	if leveldbIsCorrupted(err) {
		// The database is corrupted, and we've tried to recover it but it
		// didn't work. At this point there isn't much to do beyond dropping
		// the database and reindexing...
		l.Infoln("Database corruption detected, unable to recover. Reinitializing...")
		if err := os.RemoveAll(location); err != nil {
			return nil, &errorSuggestion{err, "failed to delete corrupted database"}
		}
		db, err = leveldb.OpenFile(location, opts)
	}
	if err != nil {
		return nil, &errorSuggestion{err, "is another instance of Syncthing running?"}
	}

	if debugEnvValue("CompactEverything", 0) != 0 {
		if err := db.CompactRange(util.Range{}); err != nil {
			l.Warnln("Compacting database:", err)
		}
	}

	return db, nil
}

func debugEnvValue(key string, def int) int {
	v, err := strconv.ParseInt(os.Getenv("STDEBUG_"+key), 10, 63)
	if err != nil {
		return def
	}
	return int(v)
}

// A "better" version of leveldb's errors.IsCorrupted.
func leveldbIsCorrupted(err error) bool {
	switch {
	case err == nil:
		return false

	case errors.IsCorrupted(err):
		return true

	case strings.Contains(err.Error(), "corrupted"):
		return true
	}

	return false
}

// dbIsLarge returns whether the estimated size of the database at location
// is large enough to warrant optimization for large databases.
func dbIsLarge(location string) bool {
	if ^uint(0)>>63 == 0 {
		// We're compiled for a 32 bit architecture. We've seen trouble with
		// large settings there.
		// (https://forum.syncthing.net/t/many-small-ldb-files-with-database-tuning/13842)
		return false
	}

	entries, err := os.ReadDir(location)
	if err != nil {
		return false
	}

	var size int64
	for _, entry := range entries {
		if entry.Name() == "LOG" {
			// don't count the size
			continue
		}
		fi, err := entry.Info()
		if err != nil {
			continue
		}
		size += fi.Size()
	}

	return size > dbLargeThreshold
}

type errorSuggestion struct {
	inner      error
	suggestion string
}

func (e *errorSuggestion) Error() string {
	return fmt.Sprintf("%s (%s)", e.inner.Error(), e.suggestion)
}

// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"fmt"
	"time"
	"log/slog"

	"github.com/syncthing/syncthing/lib/protocol"
)

// First values SQLite can't store, used by full coverage logic to store
const sqliteInt64CursorValueWhenDone = -1
const hashPrefixCeiling = 1 << 32
// Maps can't be consts in Go but this isn't meant to be modified
var folderTableCursorColumns = map[string]string{
	"blocks":     "hash",
	"blocklists": "blocklist_hash",
	"files":      "sequence",
}

type folderDB struct {
	*baseDB

	folderID string

	localDeviceIdx   int64
	deleteRetention  time.Duration
	cursorValues    map[string]int64
	chunkSizes      map[string]int64
	// used to remember where in the hash cleanups we were during the last GC triggered
	// because the device sequence changed
	// blocks and blocklists are incrementally processed to the GC must continue for them
	// to complete a full scan
	coverageFullAt map[string]int64
	// We need an approximation of the count per table to estimate the chunk processing interval
	countEstimation map[string]int64
	countValidUntil map[string]time.Time
	// Avoid to many checkpoints
	truncateInterval time.Duration
	nextTruncate     time.Time
	// Each folder decides its next cleanup Time and Service schedules it accordingly
	nextCleanup      time.Time
}

func openFolderDB(folder, path string, deleteRetention time.Duration) (*folderDB, error) {
	pragmas := []string{
		"journal_mode = WAL",
		"optimize = 0x10002",
		"auto_vacuum = INCREMENTAL",
		fmt.Sprintf("application_id = %d", applicationIDFolder),
		// This avoids blocked writes to fail immediately and especially checkpoint(TRUNCATE),
		// It depends on other connexions not locking the DB too long though (TODO)
		"busy_timeout = 5000",
		// "cache_size = -16384", // will have to test for perf (default is ~2MiB)
		// even on large folders the temp store doesn't seem used for large data, memory is faster
		"temp_store = MEMORY",
		// Don't fsync on each commit but only during checkpoints which guarantees the DB is consistent
		// although last transactions might be missing (this is however OK for Synchting)
		"synchronous = NORMAL",
		// Note: this is a max target. SQLite checkpoints might fail to keep it below depending
		// on concurrent activity
		"journal_size_limit = 8388608",
	}
	schemas := []string{
		"sql/schema/common/*",
		"sql/schema/folder/*",
	}
	migrations := []string{
		"sql/migrations/common/*",
		"sql/migrations/folder/*",
	}

	base, err := openBase(path, maxDBConns, pragmas, schemas, migrations)
	if err != nil {
		return nil, err
	}

	fdb := &folderDB{
		folderID:        folder,
		baseDB:          base,
		deleteRetention: deleteRetention,
		// *cursor*, chunkSizes and coverageFullAt are used to incrementally process GC on some tables
		cursorValues:   map[string]int64{
			"blocks":        0,
			"blocklists":    0,
			"file_names":    0,
			"file_versions": 0,
			"files":         0,
		},
		chunkSizes: map[string]int64{
			"blocks":        gcMinChunkSize,
			"blocklists":    gcMinChunkSize,
			"file_names":    gcMinChunkSize,
			"file_versions": gcMinChunkSize,
			"files":         gcMinChunkSize,
		},
		// These values are arbitrary large for their usage and mean that the GC caught up
		// other values are target to reach to end a full pass on the table
		// TODO: use constants for clarity
		coverageFullAt: map[string]int64 {
			"blocks":        hashPrefixCeiling,
			"blocklists":    hashPrefixCeiling,
			"file_names":    sqliteInt64CursorValueWhenDone,
			"file_versions": sqliteInt64CursorValueWhenDone,
			"files":         sqliteInt64CursorValueWhenDone,
		},
		countEstimation: map[string]int64 {
			"blocks":        hashPrefixCeiling,
			"blocklists":    hashPrefixCeiling,
			"file_names":    0,
			"file_versions": 0,
			"files":         0,
		},
		countValidUntil: map[string]time.Time {
			// these 2 shouldn't expire as the total hash range doesn't change
			// this is more than 100 years in the future
			// TODO: remove entries if not needed
			"blocks":        time.Now().Add(1000000 * time.Hour),
			"blocklists":    time.Now().Add(1000000 * time.Hour),
			"file_names":    time.Now().Add(-time.Hour),
			"file_versions": time.Now().Add(-time.Hour),
			"files":         time.Now().Add(-time.Hour),
		},
		truncateInterval: 24 * time.Hour, // tunable?
		nextTruncate:     time.Now().Add(24 * time.Hour),
		nextCleanup:      time.Now(),
	}

	_ = fdb.PutKV("folderID", []byte(folder))

	// Touch device IDs that should always exist and have a low index
	// numbers, and will never change
	fdb.localDeviceIdx, _ = fdb.deviceIdxLocked(protocol.LocalDeviceID)
	fdb.tplInput["LocalDeviceIdx"] = fdb.localDeviceIdx

	// the files table is repeatedly scanned to find entries to garbage collect
	// it uses conditions on name_idx, version_idx, deleted, sequence, modified and blocklist_hash
	// all of those have indexes except modified
	// This tunes the cache size according to these index sizes
	// Note: the dbstat virtual table giving actual size on disk is too slow and maybe not appropriate as it includes
	// unused space in pages. So we use an estimate of the rows count multiplied by an estimate of
	// index space used by each row (64 bit for integers, bools and timestamps, 512 bit for hash :
	// 832 bits rounded up to 1024 bits = 128 bytes
	// we count the files and blocklists tables to have a more reliable number of files
	// as files has been seen underevaluated by a large factor
	count_query := `SELECT CAST(SUBSTR(stat, 1, INSTR(stat, ' ') - 1) AS INTEGER) FROM sqlite_stat1 WHERE tbl = '%s' LIMIT 1;`
	count_estimate_files := int64(0)
	count_estimate_blocklists := int64(0)
	target_cache_size := int64(0)
	// This is performance tuning, these are allowed to fail
	err = fdb.baseDB.stmt(fmt.Sprintf(count_query, "files")).Get(&count_estimate_files)
	if err != nil {
		slog.Warn(fmt.Sprintf("%s: couldn't fetch files row count for cache tuning", fdb.logID()), "error", err)
	}
	err = fdb.baseDB.stmt(fmt.Sprintf(count_query, "blocklists")).Get(&count_estimate_files)
	if err != nil {
		slog.Warn(fmt.Sprintf("%s: couldn't fetch blocklists row count for cache tuning", fdb.logID()), "error", err)
	}
	count_estimate := max(count_estimate_files, count_estimate_blocklists)

	// Leave room for the table to grow significantly and estimates to be off
	target_cache_size = 128 * count_estimate * 2
	target_cache_size = min(target_cache_size, folderMaxCacheSize)
	// Actual size is in kB
	target_cache_size /= 1000
	if target_cache_size > 2000 {
		// "-size" is used to indicate the cache size in bytes instead of pages
		pragma := fmt.Sprintf("PRAGMA cache_size = -%d", target_cache_size)
		slog.Info(fmt.Sprintf("%s cache tuned", fdb.logID()), "size", target_cache_size)
		if _, err := fdb.baseDB.stmt(pragma).Exec(); err != nil {
			slog.Warn(fmt.Sprintf("%s: couldn't set cache size", fdb.logID()), "error", err)
		}
	}

	return fdb, nil
}

// Open the database with options suitable for the migration inserts. This
// is not a safe mode of operation for normal processing, use only for bulk
// inserts with a close afterwards.
func openFolderDBForMigration(folder, path string, deleteRetention time.Duration) (*folderDB, error) {
	pragmas := []string{
		"journal_mode = OFF",
		"foreign_keys = 0",
		"synchronous = 0",
		"locking_mode = EXCLUSIVE",
		fmt.Sprintf("application_id = %d", applicationIDFolder),
	}
	schemas := []string{
		"sql/schema/common/*",
		"sql/schema/folder/*",
	}

	base, err := openBase(path, 1, pragmas, schemas, nil)
	if err != nil {
		return nil, err
	}

	fdb := &folderDB{
		folderID:        folder,
		baseDB:          base,
		deleteRetention: deleteRetention,
	}

	// Touch device IDs that should always exist and have a low index
	// numbers, and will never change
	fdb.localDeviceIdx, _ = fdb.deviceIdxLocked(protocol.LocalDeviceID)
	fdb.tplInput["LocalDeviceIdx"] = fdb.localDeviceIdx

	return fdb, nil
}

func (fdb *folderDB) deviceIdxLocked(deviceID protocol.DeviceID) (int64, error) {
	devStr := deviceID.String()
	var idx int64
	if err := fdb.stmt(`
		INSERT INTO devices(device_id)
		VALUES (?)
		ON CONFLICT(device_id) DO UPDATE
			SET device_id = excluded.device_id
		RETURNING idx
	`).Get(&idx, devStr); err != nil {
		return 0, wrap(err)
	}

	return idx, nil
}

func (fdb *folderDB) logID() string {
	return fmt.Sprintf("%s(%s)", fdb.folderID, fdb.baseName)
}

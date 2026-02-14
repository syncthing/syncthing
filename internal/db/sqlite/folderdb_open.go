// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"fmt"
	"time"

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
	chunkSizes      map[string]int
	// used to remember where in the hash cleanups we were during the last GC triggered
	// because the device sequence changed
	// blocks and blocklists are incrementally processed to the GC must continue for them
	// to complete a full scan
	coverageFullAt map[string]int64
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
		"busy_timeout = 5000", // seems to facilitate checkpoint truncate
		"cache_size = -16384", // testing for perf
		"temp_store = MEMORY", // testing for perf
		"synchronous = NORMAL",
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
		chunkSizes: map[string]int{
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
		truncateInterval: 24 * time.Hour, // tunable?
		nextTruncate:     time.Now().Add(24 * time.Hour),
		nextCleanup:      time.Now(),
	}

	_ = fdb.PutKV("folderID", []byte(folder))
	// Note: this is a target. SQLite checkpoints might fail to keep it below depending
	// on concurrent activity
	_, _ = fdb.sql.Exec("PRAGMA journal_size_limit = 8388608")

	// Touch device IDs that should always exist and have a low index
	// numbers, and will never change
	fdb.localDeviceIdx, _ = fdb.deviceIdxLocked(protocol.LocalDeviceID)
	fdb.tplInput["LocalDeviceIdx"] = fdb.localDeviceIdx

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

func (s *folderDB) deviceIdxLocked(deviceID protocol.DeviceID) (int64, error) {
	devStr := deviceID.String()
	var idx int64
	if err := s.stmt(`
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

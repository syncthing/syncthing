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

type folderDB struct {
	*baseDB

	folderID string

	localDeviceIdx  int64
	deleteRetention time.Duration
	cursor_columns  map[string]string
	cursor_values   map[string]int
	chunk_sizes     map[string]int
	// used to remember where in the hash cleanups we were during the last GC triggered
	// because the device sequence changed
	// blocks and blocklists are incrementally processed to the GC must continue for them
	// to complete a full scan
	// 1 << 32 in these means the GC caught up
	coverage_full_at      map[string]int
	// Avoid to many checkpoints
	checkpointInterval    time.Duration
	nextCheckpoint        time.Time
}

func openFolderDB(folder, path string, deleteRetention time.Duration) (*folderDB, error) {
	pragmas := []string{
		"journal_mode = WAL",
		"optimize = 0x10002",
		"auto_vacuum = INCREMENTAL",
		fmt.Sprintf("application_id = %d", applicationIDFolder),
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
		// used to incrementally process GC on some tables
		cursor_columns:  map[string]string{
			"blocks":     "hash",
			"blocklists": "blocklist_hash",
		},
		cursor_values:   map[string]int{
			"blocks":     0,
			"blocklists": 0,
		},
		chunk_sizes: map[string]int{
			"blocks":     128,
			"blocklists": 128,
		},
		// 1 << 32 : GC caught up
		coverage_full_at: map[string]int {
			"blocks":     1 << 32,
			"blocklists": 1 << 32,
		},
		checkpointInterval: 24 * time.Hour, // tunable?
		nextCheckpoint:     time.Now().Add(24 * time.Hour),
	}

	_ = fdb.PutKV("folderID", []byte(folder))

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

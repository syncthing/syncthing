// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

type folderDB struct {
	folderID string
	*baseDB

	localDeviceIdx  int64
	deleteRetention time.Duration
}

func openFolderDB(folder, path string, deleteRetention time.Duration) (*folderDB, error) {
	pragmas := []string{
		"journal_mode = WAL",
		"optimize = 0x10002",
		"auto_vacuum = INCREMENTAL",
		"default_temp_store = MEMORY",
		"temp_store = MEMORY",
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
		"default_temp_store = MEMORY",
		"temp_store = MEMORY",
		"foreign_keys = 0",
		"synchronous = 0",
		"locking_mode = EXCLUSIVE",
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
	if _, err := s.stmt(`
		INSERT OR IGNORE INTO devices(device_id)
		VALUES (?)
	`).Exec(devStr); err != nil {
		return 0, wrap(err)
	}
	var idx int64
	if err := s.stmt(`
		SELECT idx FROM devices
		WHERE device_id = ?
	`).Get(&idx, devStr); err != nil {
		return 0, wrap(err)
	}

	return idx, nil
}

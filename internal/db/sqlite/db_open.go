// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/syncthing/syncthing/internal/db"
)

const maxDBConns = 16

type DB struct {
	pathBase        string
	deleteRetention time.Duration

	*baseDB

	folderDBsMut   sync.RWMutex
	folderDBs      map[string]*folderDB
	folderDBOpener func(folder, path string, deleteRetention time.Duration) (*folderDB, error)
}

var _ db.DB = (*DB)(nil)

type Option func(*DB)

func WithDeleteRetention(d time.Duration) Option {
	return func(s *DB) {
		s.deleteRetention = d
	}
}

func Open(path string, opts ...Option) (*DB, error) {
	pragmas := []string{
		"journal_mode = WAL",
		"optimize = 0x10002",
		"auto_vacuum = INCREMENTAL",
		"default_temp_store = MEMORY",
		"temp_store = MEMORY",
	}
	schemas := []string{
		"sql/schema/common/*",
		"sql/schema/main/*",
	}

	os.MkdirAll(path, 0o700)
	mainPath := filepath.Join(path, "main.db")
	mainBase, err := openBase(mainPath, maxDBConns, pragmas, schemas, nil)
	if err != nil {
		return nil, err
	}

	db := &DB{
		pathBase:       path,
		baseDB:         mainBase,
		folderDBs:      make(map[string]*folderDB),
		folderDBOpener: openFolderDB,
	}

	return db, nil
}

// Open the database with options suitable for the migration inserts. This
// is not a safe mode of operation for normal processing, use only for bulk
// inserts with a close afterwards.
func OpenForMigration(path string) (*DB, error) {
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
		"sql/schema/main/*",
	}

	os.MkdirAll(path, 0o700)
	mainPath := filepath.Join(path, "main.db")
	mainBase, err := openBase(mainPath, 1, pragmas, schemas, nil)
	if err != nil {
		return nil, err
	}

	db := &DB{
		pathBase:       path,
		baseDB:         mainBase,
		folderDBs:      make(map[string]*folderDB),
		folderDBOpener: openFolderDBForMigration,
	}

	// // Touch device IDs that should always exist and have a low index
	// // numbers, and will never change
	// db.localDeviceIdx, _ = db.deviceIdxLocked(protocol.LocalDeviceID)
	// db.tplInput["LocalDeviceIdx"] = db.localDeviceIdx

	return db, nil
}

func OpenTemp() (*DB, error) {
	// SQLite has a memory mode, but it works differently with concurrency
	// compared to what we need with the WAL mode. So, no memory databases
	// for now.
	dir, err := os.MkdirTemp("", "syncthing-db")
	if err != nil {
		return nil, wrap(err)
	}
	path := filepath.Join(dir, "db")
	l.Debugln("Test DB in", path)
	return Open(path)
}

func (s *DB) Close() error {
	s.folderDBsMut.Lock()
	defer s.folderDBsMut.Unlock()
	for folder, fdb := range s.folderDBs {
		fdb.Close()
		delete(s.folderDBs, folder)
	}
	return wrap(s.baseDB.Close())
}

// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/slogutil"
	"github.com/syncthing/syncthing/lib/build"
)

const (
	maxDBConns         = 16
	minDeleteRetention = 24 * time.Hour
)

type DB struct {
	*baseDB

	pathBase        string
	deleteRetention time.Duration

	folderDBsMut   sync.RWMutex
	folderDBs      map[string]*folderDB
	folderDBOpener func(folder, path string, deleteRetention time.Duration) (*folderDB, error)
}

var _ db.DB = (*DB)(nil)

type Option func(*DB)

func WithDeleteRetention(d time.Duration) Option {
	return func(s *DB) {
		if d <= 0 {
			s.deleteRetention = 0
		} else {
			s.deleteRetention = max(d, minDeleteRetention)
		}
	}
}

func Open(path string, opts ...Option) (*DB, error) {
	pragmas := []string{
		"journal_mode = WAL",
		"optimize = 0x10002",
		"auto_vacuum = INCREMENTAL",
		fmt.Sprintf("application_id = %d", applicationIDMain),
	}
	schemas := []string{
		"sql/schema/common/*",
		"sql/schema/main/*",
	}
	migrations := []string{
		"sql/migrations/common/*",
		"sql/migrations/main/*",
	}

	_ = os.MkdirAll(path, 0o700)
	initTmpDir(path)

	mainPath := filepath.Join(path, "main.db")
	mainBase, err := openBase(mainPath, maxDBConns, pragmas, schemas, migrations)
	if err != nil {
		return nil, err
	}

	db := &DB{
		pathBase:       path,
		baseDB:         mainBase,
		folderDBs:      make(map[string]*folderDB),
		folderDBOpener: openFolderDB,
	}

	for _, opt := range opts {
		opt(db)
	}

	if err := db.cleanDroppedFolders(); err != nil {
		slog.Warn("Failed to clean dropped folders", slogutil.Error(err))
	}

	if err := db.startFolderDatabases(); err != nil {
		return nil, wrap(err)
	}

	return db, nil
}

// Open the database with options suitable for the migration inserts. This
// is not a safe mode of operation for normal processing, use only for bulk
// inserts with a close afterwards.
func OpenForMigration(path string) (*DB, error) {
	pragmas := []string{
		"journal_mode = OFF",
		"foreign_keys = 0",
		"synchronous = 0",
		"locking_mode = EXCLUSIVE",
		fmt.Sprintf("application_id = %d", applicationIDMain),
	}
	schemas := []string{
		"sql/schema/common/*",
		"sql/schema/main/*",
	}
	migrations := []string{
		"sql/migrations/common/*",
		"sql/migrations/main/*",
	}

	_ = os.MkdirAll(path, 0o700)
	initTmpDir(path)

	mainPath := filepath.Join(path, "main.db")
	mainBase, err := openBase(mainPath, 1, pragmas, schemas, migrations)
	if err != nil {
		return nil, err
	}

	db := &DB{
		pathBase:       path,
		baseDB:         mainBase,
		folderDBs:      make(map[string]*folderDB),
		folderDBOpener: openFolderDBForMigration,
	}

	if err := db.cleanDroppedFolders(); err != nil {
		slog.Warn("Failed to clean dropped folders", slogutil.Error(err))
	}

	return db, nil
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

func initTmpDir(path string) {
	if build.IsWindows || build.IsDarwin || os.Getenv("SQLITE_TMPDIR") != "" {
		// Doesn't use SQLITE_TMPDIR, isn't likely to have a tiny
		// ram-backed temp directory, or already set to something.
		return
	}

	// Attempt to override the SQLite temporary directory by setting the
	// env var prior to the (first) database being opened and hence
	// SQLite becoming initialized. We set the temp dir to the same
	// place we store the database, in the hope that there will be
	// enough space there for the operations it needs to perform, as
	// opposed to /tmp and similar, on some systems.
	dbTmpDir := filepath.Join(path, ".tmp")
	if err := os.MkdirAll(dbTmpDir, 0o700); err == nil {
		os.Setenv("SQLITE_TMPDIR", dbTmpDir)
	} else {
		slog.Warn("Failed to create temp directory for SQLite", slogutil.FilePath(dbTmpDir), slogutil.Error(err))
	}
}

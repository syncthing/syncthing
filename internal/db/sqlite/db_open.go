package sqlite

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/lib/protocol"
)

func Open(path string) (*DB, error) {
	// Open the database with options to enable foreign keys and recursive
	// triggers (needed for the delete+insert triggers on row replace).
	sqlDB, err := sqlx.Open(dbDriver, path+"?"+commonOptions)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return openCommon(sqlDB)
}

func OpenMemory() (*DB, error) {
	// SQLite has a memory mode, but it works differently with concurrency
	// compared to what we need with the WAL mode. So, no memory databases
	// for now.
	dir, err := os.MkdirTemp("", "syncthing-db")
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "db")
	fmt.Println("Test DB in", path)
	return Open(path)
}

func openCommon(sqlDB *sqlx.DB) (*DB, error) {
	// Set up initial tables, indexes, triggers.
	if err := initDB(sqlDB); err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db := &DB{
		sql:      sqlDB,
		prepared: make(map[string]*sqlx.Stmt),
	}

	// Touch device IDs that should always exist and have a low index
	// numbers, and will never change
	db.localDeviceIdx, _ = db.deviceIdxLocked(protocol.LocalDeviceID)

	return db, nil
}

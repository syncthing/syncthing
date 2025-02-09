package db2

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "github.com/mattn/go-sqlite3" // register sqlite3 database driver
	"github.com/syncthing/syncthing/lib/protocol"
	"google.golang.org/protobuf/proto"
)

var initStmts = []string{
	// `PRAGMA foreign_keys = ON;`,
	`CREATE TABLE IF NOT EXISTS folders (
  		idx INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
  		folder_id TEXT NOT NULL UNIQUE
 	);`,

	`CREATE TABLE IF NOT EXISTS devices (
  		idx INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
  		device_id TEXT NOT NULL UNIQUE
 	);`,

	`CREATE TABLE IF NOT EXISTS files (
		folder_idx INTEGER NOT NULL,
		device_idx INTEGER NOT NULL,
  		sequence INTEGER NOT NULL UNIQUE,
		name TEXT NOT NULL,
  		protobuf BLOB NOT NULL,
		PRIMARY KEY(folder_idx, device_idx, sequence),
		FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
		FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE
 	);`,
	`CREATE UNIQUE INDEX IF NOT EXISTS files_name ON files (folder_idx, device_idx, name);`,

	`CREATE TABLE IF NOT EXISTS globals (
		folder_idx INTEGER NOT NULL,
		device_idx INTEGER NOT NULL,
  		file_sequence INTEGER NOT NULL,
		name TEXT NOT NULL,
		FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
		FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE,
		FOREIGN KEY(file_sequence) REFERENCES files(sequence) ON DELETE CASCADE
 	);`,
}

func Open(path string) (*DB, error) {
	var err error
	sqlDB, err := sql.Open("sqlite3", path+"?_fk=true")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	for _, stmt := range initStmts {
		if _, err := sqlDB.Exec(stmt); err != nil {
			return nil, fmt.Errorf("init statements: %s: %w", stmt, err)
		}
	}

	db := &DB{sql: sqlDB}

	// should always exist and have a low index number
	_, _ = db.deviceIdx(protocol.LocalDeviceID)

	return db, nil
}

type DB struct {
	sql *sql.DB
}

func (db *DB) Close() error {
	return db.sql.Close()
}

func (db *DB) Update(folder string, device protocol.DeviceID, fs []protocol.FileInfo) error {
	tx, err := db.sql.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelReadCommitted, ReadOnly: false})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	folderIdx, err := db.folderIdx(folder)
	if err != nil {
		return err
	}
	deviceIdx, err := db.deviceIdx(device)
	if err != nil {
		return err
	}

	seq, _ := db.querySingleInteger(`SELECT MAX(sequence) FROM files WHERE folder_idx = $1 AND device_idx = $2`, folderIdx, deviceIdx)

	for _, f := range fs {
		bs, err := proto.Marshal(f.ToWire(true))
		if err != nil {
			return err
		}
		seq++
		if _, err := tx.Exec(`INSERT OR REPLACE INTO files (folder_idx, device_idx, sequence, name, protobuf) VALUES ($1, $2, $3, $4, $5)`, folderIdx, deviceIdx, seq, f.Name, bs); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO globals (folder_idx, device_idx, file_sequence, name) VALUES ($1, $2, $3, $4)`, folderIdx, deviceIdx, seq, f.Name); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) Drop(deviceID protocol.DeviceID) error {
	tx, err := db.sql.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelReadCommitted, ReadOnly: false})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := db.sql.Exec(`DELETE FROM devices WHERE device_id = $1`, deviceID.String()); err != nil {
		return err
	}

	return tx.Commit()
}

func (db *DB) folderIdx(folderID string) (int, error) {
	if _, err := db.sql.Exec(`INSERT OR IGNORE INTO folders(folder_id) VALUES($1)`, folderID); err != nil {
		return 0, fmt.Errorf("folder idx: %w", err)
	}
	if idx, err := db.querySingleInteger(`SELECT idx FROM folders WHERE folder_id = $1`, folderID); err != nil {
		return 0, fmt.Errorf("folder idx: %w", err)
	} else {
		return idx, nil
	}
}

func (db *DB) deviceIdx(deviceID protocol.DeviceID) (int, error) {
	devStr := deviceID.String()
	if _, err := db.sql.Exec(`INSERT OR IGNORE INTO devices(device_id) VALUES($1)`, devStr); err != nil {
		return 0, fmt.Errorf("device idx: %w", err)
	}
	if idx, err := db.querySingleInteger(`SELECT idx FROM devices WHERE device_id = $1`, devStr); err != nil {
		return 0, fmt.Errorf("device idx: %w", err)
	} else {
		return idx, nil
	}
}

func (db *DB) querySingleInteger(query string, args ...any) (int, error) {
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return 0, fmt.Errorf("query single integer: %w", err)
	}
	defer rows.Close()
	return scanSingleInteger(rows)
}

func scanSingleInteger(rows *sql.Rows) (int, error) {
	if !rows.Next() {
		return 0, errors.New("single integer not found")
	}
	var idx int
	if err := rows.Scan(&idx); err != nil {
		return 0, fmt.Errorf("single integer: %w", err)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("single integer: %w", err)
	}
	return idx, nil
}

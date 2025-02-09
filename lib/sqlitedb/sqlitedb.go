package db2

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "github.com/mattn/go-sqlite3" // register sqlite3 database driver
	"github.com/syncthing/syncthing/internal/gen/bep"
	"github.com/syncthing/syncthing/lib/protocol"
	"google.golang.org/protobuf/proto"
)

var initStmts = []string{
	// `PRAGMA foreign_keys = ON;`,
	`CREATE TABLE IF NOT EXISTS folders (
  		idx INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
  		folder_id TEXT NOT NULL UNIQUE
 	) STRICT;`,

	`CREATE TABLE IF NOT EXISTS devices (
  		idx INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
  		device_id TEXT NOT NULL UNIQUE
 	) STRICT;`,

	`CREATE TABLE IF NOT EXISTS files (
		folder_idx INTEGER NOT NULL,
		device_idx INTEGER NOT NULL,
  		sequence INTEGER NOT NULL PRIMARY KEY,
		name TEXT NOT NULL,
		modified INTEGER NOT NULL, -- Unix nanos
		version TEXT NOT NULL,
		deleted INTEGER NOT NULL, -- boolean
		invalid INTEGER NOT NULL, -- boolean
  		fileinfo_protobuf BLOB NOT NULL,
		FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
		FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE
 	) STRICT;`,
	`CREATE UNIQUE INDEX IF NOT EXISTS files_name ON files (folder_idx, device_idx, name);`,

	`CREATE TABLE IF NOT EXISTS needs (
		folder_idx INTEGER NOT NULL,
		device_idx INTEGER NOT NULL,
  		file_sequence INTEGER NOT NULL,
		name TEXT NOT NULL,
		FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
		FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE,
		FOREIGN KEY(file_sequence) REFERENCES files(sequence) ON DELETE CASCADE
 	) STRICT;`,
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
		seq++
		f.Sequence = seq

		bs, err := proto.Marshal(f.ToWire(true))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR REPLACE INTO files (folder_idx, device_idx, sequence, name, modified, version, deleted, invalid, fileinfo_protobuf) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, folderIdx, deviceIdx, seq, f.Name, f.ModTime().UnixNano(), f.Version.String(), f.IsDeleted(), f.IsInvalid(), bs); err != nil {
			return err
		}
		// if _, err := tx.Exec(`INSERT INTO globals (folder_idx, device_idx, file_sequence, name) VALUES ($1, $2, $3, $4)`, folderIdx, deviceIdx, seq, f.Name); err != nil {
		// 	return err
		// }
	}

	return tx.Commit()
}

func (db *DB) Drop(device protocol.DeviceID) error {
	tx, err := db.sql.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelReadCommitted, ReadOnly: false})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := db.sql.Exec(`DELETE FROM devices WHERE device_id = $1`, device.String()); err != nil {
		return err
	}

	return tx.Commit()
}

func (db *DB) Get(folder string, device protocol.DeviceID, file string) (protocol.FileInfo, bool, error) {
	rows, err := db.sql.Query(`SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN devices d ON f.device_idx = d.idx
		INNER JOIN folders o ON f.folder_idx = o.idx
		WHERE o.folder_id = $1 AND d.device_id = $2 AND f.name = $3`, folder, device.String(), file)
	if err != nil {
		return protocol.FileInfo{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return protocol.FileInfo{}, false, nil
	}
	var bs []byte
	if err := rows.Scan(&bs); err != nil {
		return protocol.FileInfo{}, false, err
	}
	if err := rows.Err(); err != nil {
		return protocol.FileInfo{}, false, err
	}
	var bfi bep.FileInfo
	if err := proto.Unmarshal(bs, &bfi); err != nil {
		return protocol.FileInfo{}, false, err
	}
	return protocol.FileInfoFromDB(&bfi), true, nil
}

type globalEntry struct {
	sequence int64
	modified int64
	version  protocol.Vector
	deleted  bool
}

func (db *DB) GetGlobal(folder string, file string) (protocol.FileInfo, bool, error) {
	rows, err := db.sql.Query(`SELECT f.sequence, f.modified, f.version, f.deleted FROM files f
		INNER JOIN folders o ON f.folder_idx = o.idx
		WHERE f.name = $1 AND f.invalid = FALSE AND o.folder_id = $2`, file, folder)
	if err != nil {
		return protocol.FileInfo{}, false, err
	}
	defer rows.Close()
	var es []globalEntry
	if rows.Next() {
		var e globalEntry
		var verStr string
		if err := rows.Scan(&e.sequence, &e.modified, &verStr, &e.deleted); err != nil {
			return protocol.FileInfo{}, false, err
		}
		ver, err := protocol.VectorFromString(verStr)
		if err != nil {
			return protocol.FileInfo{}, false, err
		}
		e.version = ver
		es = append(es, e)
	}
	if rows.Err() != nil {
		return protocol.FileInfo{}, false, err
	}

	if len(es) == 0 {
		return protocol.FileInfo{}, false, nil
	}
	newest := 0
	for i := 1; i < len(es); i++ { // XXX simplified
		if es[i].version.GreaterEqual(es[newest].version) {
			newest = i
		}
	}
	rows.Close()

	rows, err = db.sql.Query(`SELECT fileinfo_protobuf FROM files WHERE sequence = $1 `, es[newest].sequence)
	if err != nil {
		return protocol.FileInfo{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return protocol.FileInfo{}, false, errors.New("unexpectedly found no file")
	}
	var bs []byte
	if err := rows.Scan(&bs); err != nil {
		return protocol.FileInfo{}, false, err
	}
	if err := rows.Err(); err != nil {
		return protocol.FileInfo{}, false, err
	}
	var bfi bep.FileInfo
	if err := proto.Unmarshal(bs, &bfi); err != nil {
		return protocol.FileInfo{}, false, err
	}
	return protocol.FileInfoFromDB(&bfi), true, nil
}

func (db *DB) folderIdx(folderID string) (int64, error) {
	if _, err := db.sql.Exec(`INSERT OR IGNORE INTO folders(folder_id) VALUES($1)`, folderID); err != nil {
		return 0, fmt.Errorf("folder idx: %w", err)
	}
	if idx, err := db.querySingleInteger(`SELECT idx FROM folders WHERE folder_id = $1`, folderID); err != nil {
		return 0, fmt.Errorf("folder idx: %w", err)
	} else {
		return idx, nil
	}
}

func (db *DB) deviceIdx(deviceID protocol.DeviceID) (int64, error) {
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

func (db *DB) querySingleInteger(query string, args ...any) (int64, error) {
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return 0, fmt.Errorf("query single integer: %w", err)
	}
	defer rows.Close()
	return scanSingleInteger(rows)
}

func scanSingleInteger(rows *sql.Rows) (int64, error) {
	if !rows.Next() {
		return 0, errors.New("single integer not found")
	}
	var idx int64
	if err := rows.Scan(&idx); err != nil {
		return 0, fmt.Errorf("single integer: %w", err)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("single integer: %w", err)
	}
	return idx, nil
}

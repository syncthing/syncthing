package sqlitedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"iter"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3" // register sqlite3 database driver
	"github.com/syncthing/syncthing/internal/gen/bep"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"google.golang.org/protobuf/proto"
)

var initStmts = []string{
	`CREATE TABLE IF NOT EXISTS folders (
  		idx INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
  		folder_id TEXT NOT NULL UNIQUE
 	) STRICT`,

	`CREATE TABLE IF NOT EXISTS devices (
  		idx INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
  		device_id TEXT NOT NULL UNIQUE
 	) STRICT`,

	`CREATE TABLE IF NOT EXISTS files (
		folder_idx INTEGER NOT NULL,
		device_idx INTEGER NOT NULL,
  		sequence INTEGER NOT NULL,
		name TEXT NOT NULL,
		modified INTEGER NOT NULL, -- Unix nanos
		version TEXT NOT NULL,
		deleted INTEGER NOT NULL, -- boolean
		invalid INTEGER NOT NULL, -- boolean
  		fileinfo_protobuf BLOB NOT NULL,
		PRIMARY KEY(folder_idx, device_idx, sequence),
		FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
		FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE
 	) STRICT`,
	`CREATE UNIQUE INDEX IF NOT EXISTS files_name ON files (folder_idx, device_idx, name)`,

	`CREATE TABLE IF NOT EXISTS globals (
		folder_idx INTEGER NOT NULL,
		device_idx INTEGER NOT NULL,
  		file_sequence INTEGER NOT NULL,
		name TEXT NOT NULL,
		FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE,
		FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
		FOREIGN KEY(folder_idx, device_idx, file_sequence) REFERENCES files(folder_idx, device_idx, sequence) ON DELETE CASCADE
 	) STRICT`,
	`CREATE UNIQUE INDEX IF NOT EXISTS globals_seq ON globals (folder_idx, device_idx, file_sequence)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS globals_name ON globals (folder_idx, name)`,

	`CREATE TABLE IF NOT EXISTS needs (
		folder_idx INTEGER NOT NULL,
		device_idx INTEGER NOT NULL,
  		file_sequence INTEGER, -- deliberately nullable
		name TEXT NOT NULL,
		FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE,
		FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
		FOREIGN KEY(folder_idx, device_idx, file_sequence) REFERENCES files(folder_idx, device_idx, sequence) ON DELETE CASCADE
 	) STRICT`,
	`CREATE UNIQUE INDEX IF NOT EXISTS needs_file_sequence ON needs (folder_idx, device_idx, file_sequence)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS needs_name ON needs (folder_idx, device_idx, name)`,
}

func Open(path string) (*DB, error) {
	var err error
	sqlDB, err := sqlx.Open("sqlite3", path+"?_fk=true")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	for _, stmt := range initStmts {
		if _, err := sqlDB.Exec(stmt); err != nil {
			return nil, fmt.Errorf("init statements: %s: %w", stmt, err)
		}
	}

	db := &DB{sql: sqlDB}

	// should always exist and have a low index number, and will never
	// change
	db.localDeviceIdx, _ = db.deviceIdx(protocol.LocalDeviceID)

	return db, nil
}

type DB struct {
	sql            *sqlx.DB
	localDeviceIdx int64
}

func (db *DB) Close() error {
	return wrap("close", db.sql.Close())
}

func (db *DB) Update(folder string, device protocol.DeviceID, fs []protocol.FileInfo) error {
	tx, err := db.sql.BeginTxx(context.Background(), &sql.TxOptions{Isolation: sql.LevelReadUncommitted, ReadOnly: false})
	if err != nil {
		return wrap("update", err)
	}
	defer tx.Rollback()

	folderIdx, err := db.folderIdx(folder)
	if err != nil {
		return wrap("update", err)
	}
	deviceIdx, err := db.deviceIdx(device)
	if err != nil {
		return wrap("update", err)
	}

	var seq int64
	if device == protocol.LocalDeviceID {
		_ = db.sql.Get(&seq, `SELECT MAX(sequence) FROM files WHERE folder_idx = $1 AND device_idx = $2`, folderIdx, deviceIdx)
	}

	for _, f := range fs {
		f.Name = osutil.NormalizedFilename(f.Name)
		if device == protocol.LocalDeviceID {
			seq++
			f.Sequence = seq
		}

		bs, err := proto.Marshal(f.ToWire(true))
		if err != nil {
			return wrap("update", err)
		}
		if _, err := tx.Exec(`
			INSERT OR REPLACE INTO files (folder_idx, device_idx, sequence, name, modified, version, deleted, invalid, fileinfo_protobuf)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			folderIdx, deviceIdx, f.Sequence, f.Name, f.ModTime().UnixNano(), f.Version.String(), f.IsDeleted(), f.IsInvalid(), bs); err != nil {
			return wrap("update", err)
		}
		if err := db.processNeed(tx, folder, f.Name); err != nil {
			return wrap("update", err)
		}
	}

	return wrap("update", tx.Commit())
}

func (db *DB) Drop(folder string, device protocol.DeviceID) error {
	folderIdx, err := db.folderIdx(folder)
	if err != nil {
		return wrap("drop", err)
	}
	deviceIdx, err := db.deviceIdx(device)
	if err != nil {
		return wrap("drop", err)
	}

	tx, err := db.sql.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelReadCommitted, ReadOnly: false})
	if err != nil {
		return wrap("drop", err)
	}
	defer tx.Rollback()
	if _, err := db.sql.Exec(`DELETE FROM files WHERE folder_idx = $1 AND device_idx = $2`, folderIdx, deviceIdx); err != nil {
		return wrap("drop", err)
	}
	return wrap("drop", tx.Commit())
}

func (db *DB) Get(folder string, device protocol.DeviceID, file string) (*protocol.FileInfo, bool, error) {
	file = osutil.NormalizedFilename(file)

	var bfi bep.FileInfo
	err := db.sql.Get(protoValuer(&bfi), `
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN devices d ON f.device_idx = d.idx
		INNER JOIN folders o ON f.folder_idx = o.idx
		WHERE o.folder_id = $1 AND d.device_id = $2 AND f.name = $3`,
		folder, device.String(), file)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, wrap("get", err)
	}
	fi := protocol.FileInfoFromDB(&bfi)
	return &fi, true, nil
}

func (db *DB) GetGlobal(folder string, file string) (*protocol.FileInfo, bool, error) {
	file = osutil.NormalizedFilename(file)

	var bfi bep.FileInfo
	err := db.sql.Get(protoValuer(&bfi), `
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN globals g ON f.folder_idx = g.folder_idx AND f.device_idx = g.device_idx AND f.sequence = g.file_sequence
		INNER JOIN folders o ON o.idx = g.folder_idx
		WHERE o.folder_id = $1 AND g.name = $2`, folder, file)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, wrap("getGlobal", err)
	}

	fi := protocol.FileInfoFromDB(&bfi)
	return &fi, true, nil
}

func (db *DB) Need(folder string, device protocol.DeviceID) ([]string, error) {
	var names []string
	err := db.sql.Select(&names, `
		SELECT n.name FROM needs n
		INNER JOIN folders o ON o.idx = n.folder_idx
		INNER JOIN devices d ON d.idx = n.device_idx
		WHERE o.folder_id = $1 AND d.device_id = $2`,
		folder, device.String())
	return names, wrap("need", err)
}

func (db *DB) Have(folder string, device protocol.DeviceID) iter.Seq2[*protocol.FileInfo, error] {
	beps := iterProtos[bep.FileInfo](db.sql.Queryx(`
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = $1 AND d.device_id = $2`,
		folder, device.String()))
	return iterMap(beps, func(b *bep.FileInfo) *protocol.FileInfo {
		fi := protocol.FileInfoFromDB(b)
		return &fi
	})
}

func (db *DB) HaveSequence(folder string, device protocol.DeviceID, startSeq int64) iter.Seq2[*protocol.FileInfo, error] {
	beps := iterProtos[bep.FileInfo](db.sql.Queryx(`
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = $1 AND d.device_id = $2 AND f.sequence > $3
		ORDER BY f.sequence`,
		folder, device.String(), startSeq))
	return iterMap(beps, func(b *bep.FileInfo) *protocol.FileInfo {
		fi := protocol.FileInfoFromDB(b)
		return &fi
	})
}

func (db *DB) HavePrefixed(folder string, device protocol.DeviceID, prefix string) iter.Seq2[*protocol.FileInfo, error] {
	prefix = osutil.NormalizedFilename(prefix)
	glob := prefix + "/*"
	beps := iterProtos[bep.FileInfo](db.sql.Queryx(`
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = $1 AND d.device_id = $2 AND (f.name = $3 OR f.name GLOB $4)`,
		folder, device.String(), prefix, glob))
	return iterMap(beps, func(b *bep.FileInfo) *protocol.FileInfo {
		fi := protocol.FileInfoFromDB(b)
		return &fi
	})
}

func (db *DB) folderIdx(folderID string) (int64, error) {
	if _, err := db.sql.Exec(`INSERT OR IGNORE INTO folders(folder_id) VALUES($1)`, folderID); err != nil {
		return 0, wrap("folderIdx", err)
	}
	var idx int64
	if err := db.sql.Get(&idx, `SELECT idx FROM folders WHERE folder_id = $1`, folderID); err != nil {
		return 0, wrap("folderIdx", err)
	}

	return idx, nil
}

func (db *DB) deviceIdx(deviceID protocol.DeviceID) (int64, error) {
	devStr := deviceID.String()
	if _, err := db.sql.Exec(`INSERT OR IGNORE INTO devices(device_id) VALUES($1)`, devStr); err != nil {
		return 0, wrap("deviceIdx", err)
	}
	var idx int64
	if err := db.sql.Get(&idx, `SELECT idx FROM devices WHERE device_id = $1`, devStr); err != nil {
		return 0, wrap("deviceIdx", err)
	}

	return idx, nil
}

func wrap(prefix string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

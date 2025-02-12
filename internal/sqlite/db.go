package sqlite

import (
	"context"
	"database/sql"
	"encoding/hex"
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

const flagNeed = 1 << 31 // synthetised local file that mark as needed

func Open(path string) (*DB, error) {
	// Open the database with options to enable foreign keys and recursive
	// triggers (needed for the delete+insert triggers on row replace).
	sqlDB, err := sqlx.Open("sqlite3", path+"?_fk=true&_rt=true")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Set up initial tables, indexes, triggers.
	if err := initDB(sqlDB); err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db := &DB{sql: sqlDB}

	// Touch device IDs that should always exist and have a low index
	// numbers, and will never change
	db.localDeviceIdx, _ = db.deviceIdx(protocol.LocalDeviceID)
	db.globalDeviceIdx, _ = db.deviceIdx(protocol.GlobalDeviceID)

	return db, nil
}

type DB struct {
	sql             *sqlx.DB
	localDeviceIdx  int64
	globalDeviceIdx int64
}

func (db *DB) Close() error {
	return wrap("close", db.sql.Close())
}

func (db *DB) Update(folder string, device protocol.DeviceID, fs []protocol.FileInfo) error {
	tx, err := db.sql.BeginTxx(context.Background(), &sql.TxOptions{Isolation: sql.LevelReadUncommitted, ReadOnly: false})
	if err != nil {
		return wrap("update", err)
	}
	defer tx.Rollback() //nolint:errcheck

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
		_ = db.sql.Get(&seq, `SELECT MAX(sequence) FROM files WHERE folder_idx = ? AND device_idx = ?`, folderIdx, deviceIdx)
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

		// Update the file
		if _, err := tx.Exec(`
			INSERT OR REPLACE INTO files (folder_idx, device_idx, sequence, name, type, modified, size, version, deleted, invalid, local_flags, fileinfo_protobuf)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			folderIdx, deviceIdx, f.Sequence, f.Name, f.Type, f.ModTime().UnixNano(), f.Size, f.Version.String(), f.IsDeleted(), f.IsInvalid(), f.LocalFlags, bs); err != nil {
			return wrap("update (insert file)", err)
		}

		// Update global and need
		if err := db.processNeed(tx, folderIdx, f.Name); err != nil {
			return wrap("update", err)
		}

		// Update block lists
		for _, b := range f.Blocks {
			if _, err := tx.Exec(`
			INSERT OR REPLACE INTO blocks (hash, folder_idx, device_idx, file_sequence, offset)
			VALUES ($1, $2, $3, $4, $5)`,
				hex.EncodeToString(b.Hash), folderIdx, deviceIdx, f.Sequence, b.Offset); err != nil {
				return wrap("update (insert block)", err)
			}
		}
	}

	return wrap("update", tx.Commit())
}

func (db *DB) DropNames(folder string, device protocol.DeviceID, names []string) error {
	folderIdx, err := db.folderIdx(folder)
	if err != nil {
		return wrap("remove", err)
	}
	deviceIdx, err := db.deviceIdx(device)
	if err != nil {
		return wrap("remove", err)
	}

	tx, err := db.sql.BeginTxx(context.Background(), &sql.TxOptions{Isolation: sql.LevelReadUncommitted, ReadOnly: false})
	if err != nil {
		return wrap("remove", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for _, name := range names {
		name = osutil.NormalizedFilename(name)
		if _, err := tx.Exec(`DELETE FROM files WHERE folder_idx = $1 AND device_idx = $2 AND name = $3`, folderIdx, deviceIdx, name); err != nil {
			return wrap("remove", err)
		}
		if err := db.processNeed(tx, folderIdx, name); err != nil {
			return wrap("remove", err)
		}
	}
	return wrap("remove", tx.Commit())
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
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`DELETE FROM files WHERE folder_idx = $1 AND device_idx = $2`, folderIdx, deviceIdx); err != nil {
		return wrap("drop", err)
	}
	return wrap("drop", tx.Commit())
}

func (db *DB) Local(folder string, device protocol.DeviceID, file string) (*protocol.FileInfo, bool, error) {
	file = osutil.NormalizedFilename(file)

	var bfi bep.FileInfo
	err := db.sql.Get(protoValuer(&bfi), `
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN devices d ON f.device_idx = d.idx
		INNER JOIN folders o ON f.folder_idx = o.idx
		WHERE o.folder_id = ? AND d.device_id = ? AND f.name = ? AND f.version != ""`,
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

func (db *DB) Global(folder string, file string) (*protocol.FileInfo, bool, error) {
	file = osutil.NormalizedFilename(file)

	var bfi bep.FileInfo
	err := db.sql.Get(protoValuer(&bfi), `
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		WHERE o.folder_id = ? AND f.device_idx = ? AND f.name = ?`, folder, db.globalDeviceIdx, file)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, wrap("global", err)
	}

	fi := protocol.FileInfoFromDB(&bfi)
	return &fi, true, nil
}

func (db *DB) AllNeededNames(folder string, device protocol.DeviceID) ([]string, error) {
	var names []string
	err := db.sql.Select(&names, `
		SELECT f.name FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ? AND f.local_flags & ? != 0
		`,
		folder, device.String(), flagNeed)
	return names, wrap("need", err)
}

func (db *DB) AllLocal(folder string, device protocol.DeviceID) iter.Seq2[*protocol.FileInfo, error] {
	beps := iterProtos[bep.FileInfo](db.sql.Queryx(`
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ? AND f.version != ""`,
		folder, device.String()))
	return iterMap(beps, func(b *bep.FileInfo) *protocol.FileInfo {
		fi := protocol.FileInfoFromDB(b)
		return &fi
	})
}

func (db *DB) AllLocalSequenced(folder string, device protocol.DeviceID, startSeq int64) iter.Seq2[*protocol.FileInfo, error] {
	beps := iterProtos[bep.FileInfo](db.sql.Queryx(`
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ? AND f.sequence > ? AND f.version != ""
		ORDER BY f.sequence`,
		folder, device.String(), startSeq))
	return iterMap(beps, func(b *bep.FileInfo) *protocol.FileInfo {
		fi := protocol.FileInfoFromDB(b)
		return &fi
	})
}

func (db *DB) AllLocalPrefixed(folder string, device protocol.DeviceID, prefix string) iter.Seq2[*protocol.FileInfo, error] {
	prefix = osutil.NormalizedFilename(prefix)
	glob := prefix + "/*"
	beps := iterProtos[bep.FileInfo](db.sql.Queryx(`
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ? AND (f.name = ? OR f.name GLOB ?) AND f.version != ""`,
		folder, device.String(), prefix, glob))
	return iterMap(beps, func(b *bep.FileInfo) *protocol.FileInfo {
		fi := protocol.FileInfoFromDB(b)
		return &fi
	})
}

func (db *DB) folderIdx(folderID string) (int64, error) {
	if _, err := db.sql.Exec(`INSERT OR IGNORE INTO folders(folder_id) VALUES(?)`, folderID); err != nil {
		return 0, wrap("folderIdx", err)
	}
	var idx int64
	if err := db.sql.Get(&idx, `SELECT idx FROM folders WHERE folder_id = ?`, folderID); err != nil {
		return 0, wrap("folderIdx", err)
	}

	return idx, nil
}

func (db *DB) deviceIdx(deviceID protocol.DeviceID) (int64, error) {
	devStr := deviceID.String()
	if _, err := db.sql.Exec(`INSERT OR IGNORE INTO devices(device_id) VALUES(?)`, devStr); err != nil {
		return 0, wrap("deviceIdx", err)
	}
	var idx int64
	if err := db.sql.Get(&idx, `SELECT idx FROM devices WHERE device_id = ?`, devStr); err != nil {
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

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
	"github.com/syncthing/syncthing/internal/itererr"
	olddb "github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"google.golang.org/protobuf/proto"
)

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
	defer tx.Rollback() //nolint:errcheck

	folderIdx, err := db.folderIdx(folder)
	if err != nil {
		return wrap("update", err)
	}
	deviceIdx, err := db.deviceIdx(device)
	if err != nil {
		return wrap("update", err)
	}

	for _, f := range fs {
		f.Name = osutil.NormalizedFilename(f.Name)

		var blockshash *string
		if len(f.Blocks) > 0 {
			f.BlocksHash = protocol.BlocksHash(f.Blocks)
			h := hex.EncodeToString(f.BlocksHash)
			blockshash = &h
		} else {
			f.BlocksHash = nil
		}

		if f.Deleted {
			f.LocalFlags |= protocol.FlagLocalDeleted
		}

		// Insert the file.
		//
		// If it is a remote file, set remote_sequence otherwise leave it at
		// null and marshal the FileInfo for insertion. Returns the new
		// local sequence.
		bs := []byte{} // deliberately empty but not nil
		var remoteSeq *int64
		if device != protocol.LocalDeviceID {
			remoteSeq = &f.Sequence
			bs, err = proto.Marshal(f.ToWire(true))
			if err != nil {
				return wrap("update", err)
			}
		}
		var localSeq int64
		if err := tx.Get(&localSeq, `
			INSERT OR REPLACE INTO files (folder_idx, device_idx, remote_sequence, name, type, modified, size, version, deleted, invalid, local_flags, blocks_hash, fileinfo_protobuf)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			RETURNING sequence`,
			folderIdx, deviceIdx, remoteSeq, f.Name, f.Type, f.ModTime().UnixNano(), f.Size, f.Version.String(), f.IsDeleted(), f.IsInvalid(), f.LocalFlags, blockshash, bs); err != nil {
			return wrap("update (insert file)", err)
		}

		// If the update is for the local device we only got the sequence
		// number after the insert above, so we now update the FileInfo and
		// marshal it into the row with an update.
		if device == protocol.LocalDeviceID {
			f.Sequence = localSeq
			bs, err = proto.Marshal(f.ToWire(true))
			if err != nil {
				return wrap("update", err)
			}
			if _, err := tx.Exec(`UPDATE files SET fileinfo_protobuf = ? WHERE sequence = ?`,
				bs, localSeq); err != nil {
				return wrap("update (update local file)", err)
			}
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
				hex.EncodeToString(b.Hash), folderIdx, deviceIdx, localSeq, b.Offset); err != nil {
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

func (db *DB) Drop(folder string, device protocol.DeviceID, names []string) error {
	for i := range names {
		names[i] = osutil.NormalizedFilename(names[i])
	}

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
	if _, err := tx.Exec(`DELETE FROM files WHERE folder_idx = ? AND device_idx = ? AND name in ?`, folderIdx, deviceIdx, names); err != nil {
		return wrap("drop", err)
	}
	return wrap("drop", tx.Commit())
}

func (db *DB) Local(folder string, device protocol.DeviceID, file string) (protocol.FileInfo, bool, error) {
	file = osutil.NormalizedFilename(file)

	var bfi bep.FileInfo
	err := db.sql.Get(protoValuer(&bfi), `
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN devices d ON f.device_idx = d.idx
		INNER JOIN folders o ON f.folder_idx = o.idx
		WHERE o.folder_id = ? AND d.device_id = ? AND f.name = ? AND f.version != ""`,
		folder, device.String(), file)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.FileInfo{}, false, nil
	}
	if err != nil {
		return protocol.FileInfo{}, false, wrap("get", err)
	}
	return protocol.FileInfoFromDB(&bfi), true, nil
}

func (db *DB) Global(folder string, file string) (protocol.FileInfo, bool, error) {
	file = osutil.NormalizedFilename(file)

	var bfi bep.FileInfo
	err := db.sql.Get(protoValuer(&bfi), `
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		WHERE o.folder_id = ? AND f.name = ? AND f.local_flags & ? != 0`, folder, file, protocol.FlagLocalGlobal)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.FileInfo{}, false, nil
	}
	if err != nil {
		return protocol.FileInfo{}, false, wrap("global", err)
	}

	return protocol.FileInfoFromDB(&bfi), true, nil
}

func (db *DB) Sequence(folder string, device protocol.DeviceID) int64 {
	var seq int64
	field := "sequence"
	if device != protocol.LocalDeviceID {
		field = "remote_sequence"
	}
	err := db.sql.Get(seq, fmt.Sprintf(`
		SELECT MAX(f.%s) FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device = ?`, field),
		folder, device.String())
	if errors.Is(err, sql.ErrNoRows) {
		return 0
	}
	return seq
}

func (db *DB) AllLocal(folder string, device protocol.DeviceID) iter.Seq2[*protocol.FileInfo, error] {
	beps := iterProtos[bep.FileInfo](db.sql.Queryx(`
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ? AND f.version != ""`,
		folder, device.String()))
	return itererr.Map(beps, func(b *bep.FileInfo) *protocol.FileInfo {
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
	return itererr.Map(beps, func(b *bep.FileInfo) *protocol.FileInfo {
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
	return itererr.Map(beps, func(b *bep.FileInfo) *protocol.FileInfo {
		fi := protocol.FileInfoFromDB(b)
		return &fi
	})
}

func (db *DB) AllForBlocksHash(h []byte) iter.Seq2[*protocol.FileInfo, error] {
	beps := iterProtos[bep.FileInfo](db.sql.Queryx(`
		SELECT fileinfo_protobuf FROM files
		WHERE blockshash = ?`,
		hex.EncodeToString(h)))
	return itererr.Map(beps, func(b *bep.FileInfo) *protocol.FileInfo {
		fi := protocol.FileInfoFromDB(b)
		return &fi
	})
}

type sizesRow struct {
	Type    protocol.FileInfoType
	Count   int
	Size    int64
	FlagBit int64 `db:"flag_bit"`
}

func (db *DB) LocalSize(folder string, device protocol.DeviceID) olddb.Counts {
	var res []sizesRow
	err := db.sql.Select(&res, `
		SELECT s.type, s.count, s.size, s.flag_bit FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		INNER JOIN devices d ON d.idx = s.device_idx
		WHERE o.folder_id = ? AND d.device_id = ? AND flag_bit != ?
	`, folder, device.String(), protocol.FlagLocalGlobal|protocol.FlagLocalNeeded)
	if err != nil {
		return olddb.Counts{}
	}
	all := summarizeRows(res)

	err = db.sql.Select(&res, `
		SELECT s.type, s.count, s.size, s.flag_bit FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		INNER JOIN devices d ON d.idx = s.device_idx
		WHERE o.folder_id = ? AND d.device_id = ? AND flag_bit = ?
	`, folder, device.String(), protocol.FlagLocalGlobal|protocol.FlagLocalNeeded)
	if err != nil {
		return olddb.Counts{}
	}
	doubleCounted := summarizeRows(res)

	return all.Subtract(doubleCounted)
}

func (db *DB) NeedSize(folder string, device protocol.DeviceID) olddb.Counts {
	if device == protocol.LocalDeviceID {
		return db.needSizeLocal(folder)
	}
	return db.needSizeRemote(folder, device)
}

func (db *DB) needSizeLocal(folder string) olddb.Counts {
	// The need size for the local device is the sum of entries with both
	// the global and need bit set.
	var res []sizesRow
	err := db.sql.Select(&res, `
		SELECT s.type, s.count, s.size, s.flag_bit FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		WHERE o.folder_id = ? AND flag_bit = ?
	`, folder, protocol.FlagLocalNeeded|protocol.FlagLocalGlobal)
	if err != nil {
		return olddb.Counts{}
	}
	return summarizeRows(res)
}

func (db *DB) needSizeRemote(folder string, device protocol.DeviceID) olddb.Counts {
	// The need size for a remote device is the global size minus the local
	// size plus the need size.
	var res []sizesRow
	err := db.sql.Select(&res, `
		SELECT type, count, size, flag_bit FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		INNER JOIN devices d ON d.idx = s.device_idx
		WHERE d.device_id = ? AND flag_bit = ?
	`, folder, device.String(), protocol.FlagLocalNeeded)
	if err != nil {
		panic(err)
	}
	need := summarizeRows(res)
	have := db.LocalSize(folder, device)
	global := db.GlobalSize(folder)
	return global.Subtract(have).Add(need)
}

func (db *DB) GlobalSize(folder string) olddb.Counts {
	var res []sizesRow
	err := db.sql.Select(&res, `
		SELECT s.type, s.count, s.size, s.flag_bit FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		WHERE o.folder_id = ? AND s.flag_bit = ?
	`, folder, protocol.FlagLocalGlobal)
	if err != nil {
		return olddb.Counts{}
	}
	return summarizeRows(res)
}

func (db *DB) ReceiveOnlySize(folder string) olddb.Counts {
	var res []sizesRow
	err := db.sql.Select(&res, `
		SELECT s.type, s.count, s.size, s.flag_bit FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		WHERE o.folder_id = ? AND flag_bit = ?
	`, folder, protocol.FlagLocalReceiveOnly)
	if err != nil {
		return olddb.Counts{}
	}
	return summarizeRows(res)
}

func summarizeRows(res []sizesRow) olddb.Counts {
	c := olddb.Counts{
		DeviceID: protocol.LocalDeviceID,
	}
	for _, r := range res {
		switch {
		case r.FlagBit&protocol.FlagLocalDeleted != 0:
			c.Deleted += r.Count
		case r.Type == protocol.FileInfoTypeFile:
			c.Files += r.Count
			c.Bytes += r.Size
		case r.Type == protocol.FileInfoTypeDirectory:
			c.Directories += r.Count
			c.Bytes += r.Size
		case r.Type == protocol.FileInfoTypeSymlink:
			c.Symlinks += r.Count
			c.Bytes += r.Size
		}
	}
	return c
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

package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"iter"
	"sync"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/internal/gen/bep"
	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"google.golang.org/protobuf/proto"
)

type DB struct {
	sql            *sqlx.DB
	localDeviceIdx int64
	updateLock     sync.Mutex
	prepared       map[string]*sqlx.Stmt
}

func (db *DB) Close() error {
	db.updateLock.Lock()
	defer db.updateLock.Unlock()
	return wrap("close", db.sql.Close())
}

func (db *DB) stmt(query string) (*sqlx.Stmt, error) {
	if stmt, ok := db.prepared[query]; ok {
		return stmt, nil
	}
	stmt, err := db.sql.Preparex(query)
	if err != nil {
		return nil, err
	}
	db.prepared[query] = stmt
	return stmt, nil
}

func (db *DB) Update(folder string, device protocol.DeviceID, fs []protocol.FileInfo) error {
	db.updateLock.Lock()
	defer db.updateLock.Unlock()

	tx, err := db.sql.BeginTxx(context.Background(), nil)
	if err != nil {
		return wrap("update", err)
	}
	defer tx.Rollback() //nolint:errcheck
	txp := &txPreparedStmts{Tx: tx}

	folderIdx, err := db.folderIdxLocked(folder)
	if err != nil {
		return wrap("update", err)
	}
	deviceIdx, err := db.deviceIdxLocked(device)
	if err != nil {
		return wrap("update", err)
	}

	insertFileStmt, err := txp.Preparex(`
		INSERT OR REPLACE INTO files (folder_idx, device_idx, remote_sequence, name, type, modified, size, version, deleted, invalid, local_flags, blocks_hash, fileinfo_protobuf)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING sequence`)
	if err != nil {
		return wrap("update", err)
	}
	defer insertFileStmt.Close()

	updateFileInfoStmt, err := txp.Preparex(`UPDATE files SET fileinfo_protobuf = ? WHERE sequence = ?`)
	if err != nil {
		return wrap("update", err)
	}
	defer updateFileInfoStmt.Close()

	var prevRemoteSeq int64
	for i, f := range fs {
		f.Name = osutil.NormalizedFilename(f.Name)

		var blockshash *string
		if len(f.Blocks) > 0 {
			f.BlocksHash = protocol.BlocksHash(f.Blocks)
			h := base64.RawStdEncoding.EncodeToString(f.BlocksHash)
			blockshash = &h
		} else {
			f.BlocksHash = nil
		}

		if f.Deleted {
			f.LocalFlags |= protocol.FlagLocalDeleted
		}

		if f.Type == protocol.FileInfoTypeDirectory {
			f.Size = 128 // synthetic directory size
		}

		// Insert the file.
		//
		// If it is a remote file, set remote_sequence otherwise leave it at
		// null and marshal the FileInfo for insertion. Returns the new
		// local sequence.
		bs := []byte{} // deliberately empty but not nil
		var remoteSeq *int64
		if device != protocol.LocalDeviceID {
			if i > 0 && f.Sequence == prevRemoteSeq {
				return fmt.Errorf("duplicate remote sequence number %d", prevRemoteSeq)
			}
			prevRemoteSeq = f.Sequence
			remoteSeq = &f.Sequence
			bs, err = proto.Marshal(f.ToWire(true))
			if err != nil {
				return wrap("update", err)
			}
		}
		var localSeq int64
		if err := insertFileStmt.Get(&localSeq, folderIdx, deviceIdx, remoteSeq, f.Name, f.Type, f.ModTime().UnixNano(), f.Size, f.Version.String(), f.IsDeleted(), f.IsInvalid(), f.LocalFlags, blockshash, bs); err != nil {
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
			if _, err := updateFileInfoStmt.Exec(bs, localSeq); err != nil {
				return wrap("update (update local file)", err)
			}
		}

		// Update global and need
		if err := db.recalcGlobalForFile(txp, folderIdx, f.Name); err != nil {
			return wrap("update", err)
		}

		if device == protocol.LocalDeviceID {
			// Update block lists
			if err := db.insertBlocksLocked(tx, folderIdx, deviceIdx, localSeq, f.Blocks); err != nil {
				return wrap("update", err)
			}
		}
	}

	return wrap("update", tx.Commit())
}

func (db *DB) DropFolder(folder string) error {
	db.updateLock.Lock()
	defer db.updateLock.Unlock()
	_, err := db.sql.Exec(`DELETE FROM folders WHERE folder_id = ?`, folder)
	return err
}

func (db *DB) DropDevice(device protocol.DeviceID) error {
	if device == protocol.LocalDeviceID {
		panic("bug: cannot drop local device")
	}

	db.updateLock.Lock()
	defer db.updateLock.Unlock()

	deviceIdx, err := db.deviceIdxLocked(device)
	if err != nil {
		return wrap("drop device", err)
	}

	tx, err := db.sql.BeginTxx(context.Background(), nil)
	if err != nil {
		return wrap("drop device", err)
	}
	defer tx.Rollback() //nolint:errcheck
	txp := &txPreparedStmts{Tx: tx}

	// Find all folders where the device is involved
	var folderIdxs []int64
	if err := tx.Select(&folderIdxs, `
		SELECT folder_idx
		FROM sizes
		WHERE device_idx = ? AND count > 0
		GROUP BY folder_idx`, deviceIdx); err != nil {
		return wrap("drop device", err)
	}

	// Drop the device, which cascades to delete all files etc for it
	if _, err := tx.Exec(`DELETE FROM devices WHERE device_id = ?`, device.String()); err != nil {
		return wrap("drop device", err)
	}

	// Recalc the globals for all affected folders
	for _, idx := range folderIdxs {
		if err := db.recalcGlobalForFolder(txp, idx); err != nil {
			return wrap("drop device", err)
		}
	}

	return wrap("drop device", tx.Commit())
}

func (db *DB) DropAllFiles(folder string, device protocol.DeviceID) error {
	db.updateLock.Lock()
	defer db.updateLock.Unlock()

	// This is a two part operation, first dropping all the files and then
	// recalculating the global state for the entire folder.

	folderIdx, err := db.folderIdxLocked(folder)
	if err != nil {
		return wrap("drop all files", err)
	}

	tx, err := db.sql.BeginTxx(context.Background(), nil)
	if err != nil {
		return wrap("drop all files", err)
	}
	defer tx.Rollback() //nolint:errcheck
	txp := &txPreparedStmts{Tx: tx}

	// Drop all the file entries

	if _, err := tx.Exec(`
		DELETE FROM files WHERE ROWID in (
			SELECT f.ROWID FROM files f
			INNER JOIN devices d ON f.device_idx = d.idx
			WHERE f.folder_idx = ? AND d.device_id = ?
		)
	`, folderIdx, device.String()); err != nil {
		return wrap("drop all files", err)
	}

	// Recalc global for the entire folder

	if err := db.recalcGlobalForFolder(txp, folderIdx); err != nil {
		return wrap("drop all files", err)
	}
	return wrap("drop all files", tx.Commit())
}

func (db *DB) DropFilesNamed(folder string, device protocol.DeviceID, names []string) error {
	for i := range names {
		names[i] = osutil.NormalizedFilename(names[i])
	}

	db.updateLock.Lock()
	defer db.updateLock.Unlock()

	folderIdx, err := db.folderIdxLocked(folder)
	if err != nil {
		return wrap("drop all files", err)
	}

	tx, err := db.sql.BeginTxx(context.Background(), nil)
	if err != nil {
		return wrap("drop all files", err)
	}
	defer tx.Rollback() //nolint:errcheck
	txp := &txPreparedStmts{Tx: tx}

	// Drop the named files

	query, args, err := sqlx.In(`
		DELETE FROM files WHERE ROWID in (
			SELECT f.ROWID FROM files f
			INNER JOIN devices d ON f.device_idx = d.idx
			WHERE f.folder_idx = ? AND device_id = ? AND f.name IN (?)
		)
	`, folderIdx, device.String(), names)
	if err != nil {
		return wrap("drop files named", err)
	}
	if _, err := tx.Exec(query, args...); err != nil {
		return wrap("drop files named", err)
	}

	// Recalc globals for the named files

	for _, name := range names {
		if err := db.recalcGlobalForFile(txp, folderIdx, name); err != nil {
			return wrap("drop files named", err)
		}
	}

	return wrap("drop files named", tx.Commit())
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
	return nativeFilename(protocol.FileInfoFromDB(&bfi)), true, nil
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

	return nativeFilename(protocol.FileInfoFromDB(&bfi)), true, nil
}

func (db *DB) AllGlobal(folder string) iter.Seq2[protocol.FileInfo, error] {
	beps := iterProtos[bep.FileInfo](db.sql.Queryx(`
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		WHERE o.folder_id = ? AND f.local_flags & ? != 0`,
		folder, protocol.FlagLocalGlobal))
	return itererr.Map(beps, func(b *bep.FileInfo) protocol.FileInfo {
		return nativeFilename(protocol.FileInfoFromDB(b))
	})
}

func (db *DB) AllGlobalPrefix(folder string, prefix string) iter.Seq2[protocol.FileInfo, error] {
	if prefix == "" {
		return db.AllGlobal(folder)
	}

	prefix = osutil.NormalizedFilename(prefix)
	pattern := prefix + "%"

	beps := iterProtos[bep.FileInfo](db.sql.Queryx(`
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		WHERE o.folder_id = ? AND (f.name = ? OR f.name LIKE ?) AND f.local_flags & ? != 0`,
		folder, prefix, pattern, protocol.FlagLocalGlobal))
	return itererr.Map(beps, func(b *bep.FileInfo) protocol.FileInfo {
		return nativeFilename(protocol.FileInfoFromDB(b))
	})
}

func (db *DB) Sequence(folder string, device protocol.DeviceID) (int64, error) {
	field := "sequence"
	if device != protocol.LocalDeviceID {
		field = "remote_sequence"
	}

	var res sql.NullInt64
	err := db.sql.Get(&res, fmt.Sprintf(`
		SELECT MAX(f.%s) FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ?`, field),
		folder, device.String())
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, wrap("sequence", err)
	}
	if !res.Valid {
		return 0, nil
	}
	return res.Int64, nil
}

func (db *DB) AllLocal(folder string, device protocol.DeviceID) iter.Seq2[*protocol.FileInfo, error] {
	beps := iterProtos[bep.FileInfo](db.sql.Queryx(`
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ?`,
		folder, device.String()))
	return itererr.Map(beps, func(b *bep.FileInfo) *protocol.FileInfo {
		fi := nativeFilename(protocol.FileInfoFromDB(b))
		return &fi
	})
}

func (db *DB) AllLocalSequenced(folder string, device protocol.DeviceID, startSeq int64) iter.Seq2[*protocol.FileInfo, error] {
	beps := iterProtos[bep.FileInfo](db.sql.Queryx(`
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ? AND f.sequence >= ?
		ORDER BY f.sequence`,
		folder, device.String(), startSeq))
	return itererr.Map(beps, func(b *bep.FileInfo) *protocol.FileInfo {
		fi := nativeFilename(protocol.FileInfoFromDB(b))
		return &fi
	})
}

func (db *DB) AllLocalPrefixed(folder string, device protocol.DeviceID, prefix string) iter.Seq2[*protocol.FileInfo, error] {
	if prefix == "" {
		return db.AllLocal(folder, device)
	}

	prefix = osutil.NormalizedFilename(prefix)
	pattern := prefix + "%"

	beps := iterProtos[bep.FileInfo](db.sql.Queryx(`
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ? AND (f.name = ? OR f.name LIKE ?)`,
		folder, device.String(), prefix, pattern))
	return itererr.Map(beps, func(b *bep.FileInfo) *protocol.FileInfo {
		fi := nativeFilename(protocol.FileInfoFromDB(b))
		return &fi
	})
}

func (db *DB) AllForBlocksHash(folder string, h []byte) iter.Seq2[*protocol.FileInfo, error] {
	beps := iterProtos[bep.FileInfo](db.sql.Queryx(`
		SELECT f.fileinfo_protobuf FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		WHERE o.folder_id = ? AND f.blocks_hash = ?`,
		folder, base64.RawStdEncoding.EncodeToString(h)))
	return itererr.Map(beps, func(b *bep.FileInfo) *protocol.FileInfo {
		fi := nativeFilename(protocol.FileInfoFromDB(b))
		return &fi
	})
}

type sizesRow struct {
	Type    protocol.FileInfoType
	Count   int
	Size    int64
	FlagBit int64 `db:"local_flags"`
}

func (db *DB) LocalSize(folder string, device protocol.DeviceID) Counts {
	var res []sizesRow
	extra := ""
	if device == protocol.LocalDeviceID {
		// The size counters for the local device are special, in that we
		// synthetise entries with both the Global and Need flag for files
		// that we don't currently have. We need to exlude those from the
		// local size sum.
		extra = fmt.Sprintf(" AND local_flags & %[1]d != %[1]d", protocol.FlagLocalGlobal|protocol.FlagLocalNeeded)
	}
	if err := db.sql.Select(&res, `
		SELECT s.type, s.count, s.size, s.local_flags FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		INNER JOIN devices d ON d.idx = s.device_idx
		WHERE o.folder_id = ? AND d.device_id = ?`+extra,
		folder, device.String()); err != nil {
		return Counts{}
	}
	return summarizeRows(res)
}

func (db *DB) Folders() ([]string, error) {
	var res []string
	err := db.sql.Select(&res, `SELECT folder_id FROM folders ORDER BY folder_id`)
	return res, wrap("folders", err)
}

func (db *DB) DevicesForFolder(folder string) ([]protocol.DeviceID, error) {
	var res []string
	err := db.sql.Select(&res, `
		SELECT d.device_id FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		INNER JOIN devices d ON d.idx = s.device_idx
		WHERE o.folder_id = ? AND s.count > 0 AND s.device_idx != ?
		GROUP BY d.device_id
		ORDER BY d.device_id
	`, folder, db.localDeviceIdx)
	if err != nil {
		return nil, wrap("devices for folder", err)
	}

	devs := make([]protocol.DeviceID, len(res))
	for i, s := range res {
		devs[i], err = protocol.DeviceIDFromString(s)
		if err != nil {
			return nil, err
		}
	}
	return devs, nil
}

func (db *DB) NeedSize(folder string, device protocol.DeviceID) Counts {
	if device == protocol.LocalDeviceID {
		return db.needSizeLocal(folder)
	}
	return db.needSizeRemote(folder, device)
}

func (db *DB) needSizeLocal(folder string) Counts {
	// The need size for the local device is the sum of entries with both
	// the global and need bit set.
	var res []sizesRow
	err := db.sql.Select(&res, `
		SELECT s.type, s.count, s.size, s.local_flags FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		WHERE o.folder_id = ? AND local_flags & ? = ?
	`, folder, protocol.FlagLocalNeeded|protocol.FlagLocalGlobal, protocol.FlagLocalNeeded|protocol.FlagLocalGlobal)
	if err != nil {
		return Counts{}
	}
	return summarizeRows(res)
}

func (db *DB) needSizeRemote(folder string, device protocol.DeviceID) Counts {
	// The need size for a remote device is the global size minus the local
	// size plus the need size.
	var res []sizesRow
	err := db.sql.Select(&res, `
		SELECT type, count, size, local_flags FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		INNER JOIN devices d ON d.idx = s.device_idx
		WHERE d.device_id = ? AND local_flags & ? != 0
	`, folder, device.String(), protocol.FlagLocalNeeded)
	if err != nil {
		panic(err)
	}
	need := summarizeRows(res)
	have := db.LocalSize(folder, device)
	global := db.GlobalSize(folder)
	return global.Subtract(have).Add(need)
}

func (db *DB) GlobalSize(folder string) Counts {
	// Exclude receive-only changed files from the global count (legacy
	// expectation? it's a bit weird since those files can in fact be global
	// and you can get them with GetGlobal etc.)
	var res []sizesRow
	err := db.sql.Select(&res, `
		SELECT s.type, s.count, s.size, s.local_flags FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		WHERE o.folder_id = ? AND s.local_flags & ? != 0 AND s.local_flags & ? == 0
	`, folder, protocol.FlagLocalGlobal, protocol.FlagLocalReceiveOnly)
	if err != nil {
		return Counts{}
	}
	return summarizeRows(res)
}

func (db *DB) ReceiveOnlySize(folder string) Counts {
	var res []sizesRow
	err := db.sql.Select(&res, `
		SELECT s.type, s.count, s.size, s.local_flags FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		WHERE o.folder_id = ? AND local_flags & ? != 0
	`, folder, protocol.FlagLocalReceiveOnly)
	if err != nil {
		return Counts{}
	}
	return summarizeRows(res)
}

func summarizeRows(res []sizesRow) Counts {
	c := Counts{
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

func (db *DB) folderIdxLocked(folderID string) (int64, error) {
	if _, err := db.sql.Exec(`INSERT OR IGNORE INTO folders(folder_id) VALUES(?)`, folderID); err != nil {
		return 0, wrap("folderIdx", err)
	}
	var idx int64
	if err := db.sql.Get(&idx, `SELECT idx FROM folders WHERE folder_id = ?`, folderID); err != nil {
		return 0, wrap("folderIdx", err)
	}

	return idx, nil
}

func (db *DB) deviceIdxLocked(deviceID protocol.DeviceID) (int64, error) {
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

// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/internal/gen/dbproto"
	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/internal/slogutil"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/sliceutil"
	"google.golang.org/protobuf/proto"
)

const (
	// Arbitrarily chosen values for checkpoint frequency....
	updatePointsPerFile   = 100
	updatePointsPerBlock  = 1
	updatePointsThreshold = 250_000
)

func (s *folderDB) Update(device protocol.DeviceID, fs []protocol.FileInfo) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()

	deviceIdx, err := s.deviceIdxLocked(device)
	if err != nil {
		return wrap(err)
	}

	tx, err := s.sql.BeginTxx(context.Background(), nil)
	if err != nil {
		return wrap(err)
	}
	defer tx.Rollback() //nolint:errcheck
	txp := &txPreparedStmts{Tx: tx}

	//nolint:sqlclosecheck
	insertNameStmt, err := txp.Preparex(`
		INSERT INTO file_names(name)
		VALUES (?)
		ON CONFLICT(name) DO UPDATE
			SET name = excluded.name
		RETURNING idx
	`)
	if err != nil {
		return wrap(err, "prepare insert name")
	}

	//nolint:sqlclosecheck
	insertVersionStmt, err := txp.Preparex(`
		INSERT INTO file_versions (version)
		VALUES (?)
		ON CONFLICT(version) DO UPDATE
			SET version = excluded.version
		RETURNING idx
	`)
	if err != nil {
		return wrap(err, "prepare insert version")
	}

	//nolint:sqlclosecheck
	insertFileStmt, err := txp.Preparex(`
		INSERT OR REPLACE INTO files (device_idx, remote_sequence, type, modified, size, deleted, local_flags, blocklist_hash, name_idx, version_idx)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING sequence
	`)
	if err != nil {
		return wrap(err, "prepare insert file")
	}

	//nolint:sqlclosecheck
	insertFileInfoStmt, err := txp.Preparex(`
		INSERT INTO fileinfos (sequence, fiprotobuf)
		VALUES (?, ?)
	`)
	if err != nil {
		return wrap(err, "prepare insert fileinfo")
	}

	//nolint:sqlclosecheck
	insertBlockListStmt, err := txp.Preparex(`
		INSERT OR IGNORE INTO blocklists (blocklist_hash, blprotobuf)
		VALUES (?, ?)
	`)
	if err != nil {
		return wrap(err, "prepare insert blocklist")
	}

	var prevRemoteSeq int64
	for i, f := range fs {
		f.Name = osutil.NormalizedFilename(f.Name)

		var blockshash *[]byte
		if len(f.Blocks) > 0 {
			f.BlocksHash = protocol.BlocksHash(f.Blocks)
			blockshash = &f.BlocksHash
		} else {
			f.BlocksHash = nil
		}

		if f.Type == protocol.FileInfoTypeDirectory {
			f.Size = 128 // synthetic directory size
		}

		// Insert the file.
		//
		// If it is a remote file, set remote_sequence otherwise leave it at
		// null. Returns the new local sequence.
		var remoteSeq *int64
		if device != protocol.LocalDeviceID {
			if i > 0 && f.Sequence == prevRemoteSeq {
				return fmt.Errorf("duplicate remote sequence number %d", prevRemoteSeq)
			}
			prevRemoteSeq = f.Sequence
			remoteSeq = &f.Sequence
		}

		var nameIdx int64
		if err := insertNameStmt.Get(&nameIdx, f.Name); err != nil {
			return wrap(err, "insert name")
		}

		var versionIdx int64
		if err := insertVersionStmt.Get(&versionIdx, f.Version.String()); err != nil {
			return wrap(err, "insert version")
		}

		var localSeq int64
		if err := insertFileStmt.Get(&localSeq, deviceIdx, remoteSeq, f.Type, f.ModTime().UnixNano(), f.Size, f.IsDeleted(), f.LocalFlags, blockshash, nameIdx, versionIdx); err != nil {
			return wrap(err, "insert file")
		}

		if len(f.Blocks) > 0 {
			// Indirect the block list
			blocks := sliceutil.Map(f.Blocks, protocol.BlockInfo.ToWire)
			bs, err := proto.Marshal(&dbproto.BlockList{Blocks: blocks})
			if err != nil {
				return wrap(err, "marshal blocklist")
			}
			if _, err := insertBlockListStmt.Exec(f.BlocksHash, bs); err != nil {
				return wrap(err, "insert blocklist")
			} else if device == protocol.LocalDeviceID {
				// Insert all blocks
				if err := s.insertBlocksLocked(txp, f.BlocksHash, f.Blocks); err != nil {
					return wrap(err, "insert blocks")
				}
			}

			f.Blocks = nil
		}

		// Insert the fileinfo
		if device == protocol.LocalDeviceID {
			f.Sequence = localSeq
		}
		bs, err := proto.Marshal(f.ToWire(true))
		if err != nil {
			return wrap(err, "marshal fileinfo")
		}
		if _, err := insertFileInfoStmt.Exec(localSeq, bs); err != nil {
			return wrap(err, "insert fileinfo")
		}

		// Update global and need
		if err := s.recalcGlobalForFile(txp, f.Name); err != nil {
			return wrap(err)
		}
	}

	if err := tx.Commit(); err != nil {
		return wrap(err)
	}

	s.periodicCheckpointLocked(fs)
	return nil
}

func (s *folderDB) DropDevice(device protocol.DeviceID) error {
	if device == protocol.LocalDeviceID {
		panic("bug: cannot drop local device")
	}

	s.updateLock.Lock()
	defer s.updateLock.Unlock()

	tx, err := s.sql.BeginTxx(context.Background(), nil)
	if err != nil {
		return wrap(err)
	}
	defer tx.Rollback() //nolint:errcheck
	txp := &txPreparedStmts{Tx: tx}

	// Drop the device, which cascades to delete all files etc for it
	if _, err := tx.Exec(`DELETE FROM devices WHERE device_id = ?`, device.String()); err != nil {
		return wrap(err)
	}

	// Recalc the globals for all affected folders
	if err := s.recalcGlobalForFolder(txp); err != nil {
		return wrap(err)
	}

	return wrap(tx.Commit())
}

func (s *folderDB) DropAllFiles(device protocol.DeviceID) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()

	// This is a two part operation, first dropping all the files and then
	// recalculating the global state for the entire folder.

	deviceIdx, err := s.deviceIdxLocked(device)
	if err != nil {
		return wrap(err)
	}

	tx, err := s.sql.BeginTxx(context.Background(), nil)
	if err != nil {
		return wrap(err)
	}
	defer tx.Rollback() //nolint:errcheck
	txp := &txPreparedStmts{Tx: tx}

	// Drop all the file entries

	result, err := tx.Exec(`
		DELETE FROM files
		WHERE device_idx = ?
	`, deviceIdx)
	if err != nil {
		return wrap(err)
	}
	if n, err := result.RowsAffected(); err == nil && n == 0 {
		// The delete affected no rows, so we don't need to redo the entire
		// global/need calculation.
		return wrap(tx.Commit())
	}

	// Recalc global for the entire folder

	if err := s.recalcGlobalForFolder(txp); err != nil {
		return wrap(err)
	}
	return wrap(tx.Commit())
}

func (s *folderDB) DropFilesNamed(device protocol.DeviceID, names []string) error {
	for i := range names {
		names[i] = osutil.NormalizedFilename(names[i])
	}

	s.updateLock.Lock()
	defer s.updateLock.Unlock()

	deviceIdx, err := s.deviceIdxLocked(device)
	if err != nil {
		return wrap(err)
	}

	tx, err := s.sql.BeginTxx(context.Background(), nil)
	if err != nil {
		return wrap(err)
	}
	defer tx.Rollback() //nolint:errcheck
	txp := &txPreparedStmts{Tx: tx}

	// Drop the named files

	query, args, err := sqlx.In(`
		DELETE FROM files
		WHERE device_idx = ? AND name_idx IN (
			SELECT idx FROM file_names WHERE name IN (?)
		)
	`, deviceIdx, names)
	if err != nil {
		return wrap(err)
	}
	if _, err := tx.Exec(query, args...); err != nil {
		return wrap(err)
	}

	// Recalc globals for the named files

	for _, name := range names {
		if err := s.recalcGlobalForFile(txp, name); err != nil {
			return wrap(err)
		}
	}

	return wrap(tx.Commit())
}

func (*folderDB) insertBlocksLocked(tx *txPreparedStmts, blocklistHash []byte, blocks []protocol.BlockInfo) error {
	if len(blocks) == 0 {
		return nil
	}
	bs := make([]map[string]any, len(blocks))
	for i, b := range blocks {
		bs[i] = map[string]any{
			"hash":           b.Hash,
			"blocklist_hash": blocklistHash,
			"idx":            i,
			"offset":         b.Offset,
			"size":           b.Size,
		}
	}

	// Very large block lists (>8000 blocks) result in "too many variables"
	// error. Chunk it to a reasonable size.
	for chunk := range slices.Chunk(bs, 1000) {
		if _, err := tx.NamedExec(`
			INSERT OR IGNORE INTO blocks (hash, blocklist_hash, idx, offset, size)
			VALUES (:hash, :blocklist_hash, :idx, :offset, :size)
		`, chunk); err != nil {
			return wrap(err)
		}
	}
	return nil
}

func (s *folderDB) recalcGlobalForFolder(txp *txPreparedStmts) error {
	// Select files where there is no global, those are the ones we need to
	// recalculate.
	//nolint:sqlclosecheck
	namesStmt, err := txp.Preparex(`
		SELECT n.name FROM files f
		INNER JOIN file_names n ON n.idx = f.name_idx
		WHERE NOT EXISTS (
			SELECT 1 FROM files g
			WHERE g.name_idx = f.name_idx AND g.local_flags & ? != 0
		)
		GROUP BY n.name
	`)
	if err != nil {
		return wrap(err)
	}
	rows, err := namesStmt.Queryx(protocol.FlagLocalGlobal)
	if err != nil {
		return wrap(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return wrap(err)
		}
		if err := s.recalcGlobalForFile(txp, name); err != nil {
			return wrap(err)
		}
	}
	return wrap(rows.Err())
}

func (s *folderDB) recalcGlobalForFile(txp *txPreparedStmts, file string) error {
	//nolint:sqlclosecheck
	selStmt, err := txp.Preparex(`
		SELECT n.name, f.device_idx, f.sequence, f.modified, v.version, f.deleted, f.local_flags FROM files f
		INNER JOIN file_versions v ON v.idx = f.version_idx
		INNER JOIN file_names n ON n.idx = f.name_idx
		WHERE n.name = ?
	`)
	if err != nil {
		return wrap(err, "prepare select")
	}
	es, err := itererr.Collect(iterStructs[fileRow](selStmt.Queryx(file)))
	if err != nil {
		return wrap(err)
	}
	if len(es) == 0 {
		// shouldn't happen
		return nil
	}

	// Sort the entries; the global entry is at the head of the list
	slices.SortFunc(es, fileRow.Compare)

	// The global version is the first one in the list that is not invalid,
	// or just the first one in the list if all are invalid.
	var global fileRow
	globIdx := slices.IndexFunc(es, func(e fileRow) bool { return !e.IsInvalid() })
	if globIdx < 0 {
		globIdx = 0
	}
	global = es[globIdx]

	// We "have" the file if the position in the list of versions is at the
	// global version or better, or if the version is the same as the global
	// file (we might be further down the list due to invalid flags), or if
	// the global is deleted and we don't have it at all...
	localIdx := slices.IndexFunc(es, func(e fileRow) bool { return e.DeviceIdx == s.localDeviceIdx })
	hasLocal := localIdx >= 0 && localIdx <= globIdx || // have a better or equal version
		localIdx >= 0 && es[localIdx].Version.Equal(global.Version.Vector) || // have an equal version but invalid/ignored
		localIdx < 0 && global.Deleted // missing it, but the global is also deleted

	// Set the global flag on the global entry. Set the need flag if the
	// local device needs this file, unless it's invalid.
	global.LocalFlags |= protocol.FlagLocalGlobal
	if hasLocal || global.IsInvalid() {
		global.LocalFlags &= ^protocol.FlagLocalNeeded
	} else {
		global.LocalFlags |= protocol.FlagLocalNeeded
	}
	//nolint:sqlclosecheck
	upStmt, err := txp.Preparex(`
		UPDATE files SET local_flags = ?
		WHERE device_idx = ? AND sequence = ?
	`)
	if err != nil {
		return wrap(err)
	}
	if _, err := upStmt.Exec(global.LocalFlags, global.DeviceIdx, global.Sequence); err != nil {
		return wrap(err)
	}

	// Clear the need and global flags on all other entries
	//nolint:sqlclosecheck
	upStmt, err = txp.Preparex(`
		UPDATE files SET local_flags = local_flags & ?
		WHERE name_idx = (SELECT idx FROM file_names WHERE name = ?) AND sequence != ? AND local_flags & ? != 0
	`)
	if err != nil {
		return wrap(err, "prepare update")
	}
	if _, err := upStmt.Exec(^(protocol.FlagLocalNeeded | protocol.FlagLocalGlobal), global.Name, global.Sequence, protocol.FlagLocalNeeded|protocol.FlagLocalGlobal); err != nil {
		return wrap(err)
	}

	return nil
}

func (s *DB) folderIdxLocked(folderID string) (int64, error) {
	if _, err := s.stmt(`
		INSERT OR IGNORE INTO folders(folder_id)
		VALUES (?)
	`).Exec(folderID); err != nil {
		return 0, wrap(err)
	}
	var idx int64
	if err := s.stmt(`
		SELECT idx FROM folders
		WHERE folder_id = ?
	`).Get(&idx, folderID); err != nil {
		return 0, wrap(err)
	}

	return idx, nil
}

type fileRow struct {
	Name       string
	Version    dbVector
	DeviceIdx  int64 `db:"device_idx"`
	Sequence   int64
	Modified   int64
	Size       int64
	LocalFlags protocol.FlagLocal `db:"local_flags"`
	Deleted    bool
}

func (e fileRow) Compare(other fileRow) int {
	// From FileInfo.WinsConflict
	vc := e.Version.Compare(other.Version.Vector)
	switch vc {
	case protocol.Equal:
		if e.IsInvalid() != other.IsInvalid() {
			if e.IsInvalid() {
				return 1
			}
			return -1
		}

		// Compare the device ID index, lower is better. This is only
		// deterministic to the extent that LocalDeviceID will always be the
		// lowest one, order between remote devices is random (and
		// irrelevant).
		return cmp.Compare(e.DeviceIdx, other.DeviceIdx)
	case protocol.Greater: // we are newer
		return -1
	case protocol.Lesser: // we are older
		return 1
	case protocol.ConcurrentGreater, protocol.ConcurrentLesser: // there is a conflict
		if e.IsInvalid() != other.IsInvalid() {
			if e.IsInvalid() { // we are invalid, we lose
				return 1
			}
			return -1 // they are invalid, we win
		}
		if d := cmp.Compare(e.Modified, other.Modified); d != 0 {
			return -d // positive d means we were newer, so we win (negative return)
		}
		if vc == protocol.ConcurrentGreater {
			return -1 // we have a better device ID, we win
		}
		return 1 // they win
	default:
		return 0
	}
}

func (e fileRow) IsInvalid() bool {
	return e.LocalFlags.IsInvalid()
}

func (s *folderDB) periodicCheckpointLocked(fs []protocol.FileInfo) {
	// Induce periodic checkpoints. We add points for each file and block,
	// and checkpoint when we've written more than a threshold of points.
	// This ensures we do not go too long without a checkpoint, while also
	// not doing it incessantly for every update.
	s.updatePoints += updatePointsPerFile * len(fs)
	for _, f := range fs {
		s.updatePoints += len(f.Blocks) * updatePointsPerBlock
	}
	if s.updatePoints > updatePointsThreshold {
		conn, err := s.sql.Conn(context.Background())
		if err != nil {
			slog.Debug("Connection error", slog.String("db", s.baseName), slogutil.Error(err))
			return
		}
		defer conn.Close()
		if _, err := conn.ExecContext(context.Background(), `PRAGMA journal_size_limit = 8388608`); err != nil {
			slog.Debug("PRAGMA journal_size_limit error", slog.String("db", s.baseName), slogutil.Error(err))
		}

		// Every 50th checkpoint becomes a truncate, in an effort to bring
		// down the size now and then.
		checkpointType := "RESTART"
		if s.checkpointsCount > 50 {
			checkpointType = "TRUNCATE"
		}
		cmd := fmt.Sprintf(`PRAGMA wal_checkpoint(%s)`, checkpointType)
		row := conn.QueryRowContext(context.Background(), cmd)

		var res, modified, moved int
		if row.Err() != nil {
			slog.Debug("Command error", slog.String("db", s.baseName), slog.String("cmd", cmd), slogutil.Error(err))
		} else if err := row.Scan(&res, &modified, &moved); err != nil {
			slog.Debug("Command scan error", slog.String("db", s.baseName), slog.String("cmd", cmd), slogutil.Error(err))
		} else {
			slog.Debug("Checkpoint result", "db", s.baseName, "checkpointscount", s.checkpointsCount, "updatepoints", s.updatePoints, "res", res, "modified", modified, "moved", moved)
		}

		// Reset the truncate counter when a truncate succeeded. If it
		// failed, we'll keep trying it until we succeed. Increase it faster
		// when we fail to checkpoint, as it's more likely the WAL is
		// growing and will need truncation when we get out of this state.
		switch {
		case res == 1:
			s.checkpointsCount += 10
		case res == 0 && checkpointType == "TRUNCATE":
			s.checkpointsCount = 0
		default:
			s.checkpointsCount++
		}
		s.updatePoints = 0
	}
}

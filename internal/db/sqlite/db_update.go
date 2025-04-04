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
	"runtime"
	"slices"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/internal/gen/dbproto"
	"github.com/syncthing/syncthing/internal/itererr"
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

func (s *DB) Update(folder string, device protocol.DeviceID, fs []protocol.FileInfo) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()

	folderIdx, err := s.folderIdxLocked(folder)
	if err != nil {
		return wrap(err)
	}
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
	insertFileStmt, err := txp.Preparex(`
		INSERT OR REPLACE INTO files (folder_idx, device_idx, remote_sequence, name, type, modified, size, version, deleted, invalid, local_flags, blocklist_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		var localSeq int64
		if err := insertFileStmt.Get(&localSeq, folderIdx, deviceIdx, remoteSeq, f.Name, f.Type, f.ModTime().UnixNano(), f.Size, f.Version.String(), f.IsDeleted(), f.IsInvalid(), f.LocalFlags, blockshash); err != nil {
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
			}

			if device == protocol.LocalDeviceID {
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
		if err := s.recalcGlobalForFile(txp, folderIdx, f.Name); err != nil {
			return wrap(err)
		}
	}

	if err := tx.Commit(); err != nil {
		return wrap(err)
	}

	s.periodicCheckpointLocked(fs)
	return nil
}

func (s *DB) DropFolder(folder string) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()
	_, err := s.stmt(`
		DELETE FROM folders
		WHERE folder_id = ?
	`).Exec(folder)
	return wrap(err)
}

func (s *DB) DropDevice(device protocol.DeviceID) error {
	if device == protocol.LocalDeviceID {
		panic("bug: cannot drop local device")
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

	// Find all folders where the device is involved
	var folderIdxs []int64
	if err := tx.Select(&folderIdxs, `
		SELECT folder_idx
		FROM counts
		WHERE device_idx = ? AND count > 0
		GROUP BY folder_idx
	`, deviceIdx); err != nil {
		return wrap(err)
	}

	// Drop the device, which cascades to delete all files etc for it
	if _, err := tx.Exec(`DELETE FROM devices WHERE device_id = ?`, device.String()); err != nil {
		return wrap(err)
	}

	// Recalc the globals for all affected folders
	for _, idx := range folderIdxs {
		if err := s.recalcGlobalForFolder(txp, idx); err != nil {
			return wrap(err)
		}
	}

	return wrap(tx.Commit())
}

func (s *DB) DropAllFiles(folder string, device protocol.DeviceID) error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()

	// This is a two part operation, first dropping all the files and then
	// recalculating the global state for the entire folder.

	folderIdx, err := s.folderIdxLocked(folder)
	if err != nil {
		return wrap(err)
	}
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
		WHERE folder_idx = ? AND device_idx = ?
	`, folderIdx, deviceIdx)
	if err != nil {
		return wrap(err)
	}
	if n, err := result.RowsAffected(); err == nil && n == 0 {
		// The delete affected no rows, so we don't need to redo the entire
		// global/need calculation.
		return wrap(tx.Commit())
	}

	// Recalc global for the entire folder

	if err := s.recalcGlobalForFolder(txp, folderIdx); err != nil {
		return wrap(err)
	}
	return wrap(tx.Commit())
}

func (s *DB) DropFilesNamed(folder string, device protocol.DeviceID, names []string) error {
	for i := range names {
		names[i] = osutil.NormalizedFilename(names[i])
	}

	s.updateLock.Lock()
	defer s.updateLock.Unlock()

	folderIdx, err := s.folderIdxLocked(folder)
	if err != nil {
		return wrap(err)
	}
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
		WHERE folder_idx = ? AND device_idx = ? AND name IN (?)
	`, folderIdx, deviceIdx, names)
	if err != nil {
		return wrap(err)
	}
	if _, err := tx.Exec(query, args...); err != nil {
		return wrap(err)
	}

	// Recalc globals for the named files

	for _, name := range names {
		if err := s.recalcGlobalForFile(txp, folderIdx, name); err != nil {
			return wrap(err)
		}
	}

	return wrap(tx.Commit())
}

func (*DB) insertBlocksLocked(tx *txPreparedStmts, blocklistHash []byte, blocks []protocol.BlockInfo) error {
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

func (s *DB) recalcGlobalForFolder(txp *txPreparedStmts, folderIdx int64) error {
	// Select files where there is no global, those are the ones we need to
	// recalculate.
	//nolint:sqlclosecheck
	namesStmt, err := txp.Preparex(`
		SELECT f.name FROM files f
		WHERE f.folder_idx = ? AND NOT EXISTS (
			SELECT 1 FROM files g
			WHERE g.folder_idx = ? AND g.name = f.name AND g.local_flags & ? != 0
		)
		GROUP BY name
	`)
	if err != nil {
		return wrap(err)
	}
	rows, err := namesStmt.Queryx(folderIdx, folderIdx, protocol.FlagLocalGlobal)
	if err != nil {
		return wrap(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return wrap(err)
		}
		if err := s.recalcGlobalForFile(txp, folderIdx, name); err != nil {
			return wrap(err)
		}
	}
	return wrap(rows.Err())
}

func (s *DB) recalcGlobalForFile(txp *txPreparedStmts, folderIdx int64, file string) error {
	//nolint:sqlclosecheck
	selStmt, err := txp.Preparex(`
		SELECT name, folder_idx, device_idx, sequence, modified, version, deleted, invalid, local_flags FROM files
		WHERE folder_idx = ? AND name = ?
	`)
	if err != nil {
		return wrap(err)
	}
	es, err := itererr.Collect(iterStructs[fileRow](selStmt.Queryx(folderIdx, file)))
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
	globIdx := slices.IndexFunc(es, func(e fileRow) bool { return !e.Invalid })
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
	if hasLocal || global.Invalid {
		global.LocalFlags &= ^protocol.FlagLocalNeeded
	} else {
		global.LocalFlags |= protocol.FlagLocalNeeded
	}
	//nolint:sqlclosecheck
	upStmt, err := txp.Prepare(`
		UPDATE files SET local_flags = ?
		WHERE folder_idx = ? AND device_idx = ? AND sequence = ?
	`)
	if err != nil {
		return wrap(err)
	}
	if _, err := upStmt.Exec(global.LocalFlags, global.FolderIdx, global.DeviceIdx, global.Sequence); err != nil {
		return wrap(err)
	}

	// Clear the need and global flags on all other entries
	//nolint:sqlclosecheck
	upStmt, err = txp.Prepare(`
		UPDATE files SET local_flags = local_flags & ?
		WHERE folder_idx = ? AND name = ? AND sequence != ? AND local_flags & ? != 0
	`)
	if err != nil {
		return wrap(err)
	}
	if _, err := upStmt.Exec(^(protocol.FlagLocalNeeded | protocol.FlagLocalGlobal), folderIdx, global.Name, global.Sequence, protocol.FlagLocalNeeded|protocol.FlagLocalGlobal); err != nil {
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

func (s *DB) deviceIdxLocked(deviceID protocol.DeviceID) (int64, error) {
	devStr := deviceID.String()
	if _, err := s.stmt(`
		INSERT OR IGNORE INTO devices(device_id)
		VALUES (?)
	`).Exec(devStr); err != nil {
		return 0, wrap(err)
	}
	var idx int64
	if err := s.stmt(`
		SELECT idx FROM devices
		WHERE device_id = ?
	`).Get(&idx, devStr); err != nil {
		return 0, wrap(err)
	}

	return idx, nil
}

// wrap returns the error wrapped with the calling function name and
// optional extra context strings as prefix. A nil error wraps to nil.
func wrap(err error, context ...string) error {
	if err == nil {
		return nil
	}

	prefix := "error"
	pc, _, _, ok := runtime.Caller(1)
	details := runtime.FuncForPC(pc)
	if ok && details != nil {
		prefix = strings.ToLower(details.Name())
		if dotIdx := strings.LastIndex(prefix, "."); dotIdx > 0 {
			prefix = prefix[dotIdx+1:]
		}
	}

	if len(context) > 0 {
		for i := range context {
			context[i] = strings.TrimSpace(context[i])
		}
		extra := strings.Join(context, ", ")
		return fmt.Errorf("%s (%s): %w", prefix, extra, err)
	}

	return fmt.Errorf("%s: %w", prefix, err)
}

type fileRow struct {
	Name       string
	Version    dbVector
	FolderIdx  int64 `db:"folder_idx"`
	DeviceIdx  int64 `db:"device_idx"`
	Sequence   int64
	Modified   int64
	Size       int64
	LocalFlags int64 `db:"local_flags"`
	Deleted    bool
	Invalid    bool
}

func (e fileRow) Compare(other fileRow) int {
	// From FileInfo.WinsConflict
	vc := e.Version.Vector.Compare(other.Version.Vector)
	switch vc {
	case protocol.Equal:
		if e.Invalid != other.Invalid {
			if e.Invalid {
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
		if e.Invalid != other.Invalid {
			if e.Invalid { // we are invalid, we lose
				return 1
			}
			return -1 // they are invalid, we win
		}
		if e.Deleted != other.Deleted {
			if e.Deleted { // we are deleted, we lose
				return 1
			}
			return -1 // they are deleted, we win
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

func (s *DB) periodicCheckpointLocked(fs []protocol.FileInfo) {
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
			l.Debugln("conn:", err)
			return
		}
		defer conn.Close()
		if _, err := conn.ExecContext(context.Background(), `PRAGMA journal_size_limit = 67108864`); err != nil {
			l.Debugln("PRAGMA journal_size_limit:", err)
		}
		row := conn.QueryRowContext(context.Background(), `PRAGMA wal_checkpoint(RESTART)`)
		var res, modified, moved int
		if row.Err() != nil {
			l.Debugln("PRAGMA wal_checkpoint(RESTART):", err)
		} else if err := row.Scan(&res, &modified, &moved); err != nil {
			l.Debugln("PRAGMA wal_checkpoint(RESTART) (scan):", err)
		} else {
			l.Debugln("checkpoint at", s.updatePoints, "returned", res, modified, moved)
		}
		s.updatePoints = 0
	}
}

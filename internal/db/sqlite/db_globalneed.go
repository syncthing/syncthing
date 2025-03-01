package sqlite

import (
	"cmp"
	"database/sql"
	"errors"
	"fmt"
	"iter"
	"slices"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
)

type fileRow struct {
	Name       string
	FolderIdx  int64 `db:"folder_idx"`
	DeviceIdx  int64 `db:"device_idx"`
	Sequence   int64
	Modified   int64
	Size       int64
	Version    dbVector
	Deleted    bool
	Invalid    bool
	LocalFlags int64 `db:"local_flags"`
}

func (s *DB) AllNeededNames(folder string, device protocol.DeviceID, order config.PullOrder, limit int) iter.Seq2[string, error] {
	var orderBy string
	switch order {
	case config.PullOrderRandom:
		orderBy = "ORDER BY RANDOM()"
	case config.PullOrderAlphabetic:
		orderBy = "ORDER BY g.name ASC"
	case config.PullOrderSmallestFirst:
		orderBy = "ORDER BY g.size ASC"
	case config.PullOrderLargestFirst:
		orderBy = "ORDER BY g.size DESC"
	case config.PullOrderOldestFirst:
		orderBy = "ORDER BY g.modified ASC"
	case config.PullOrderNewestFirst:
		orderBy = "ORDER BY g.modified DESC"
	}

	var limitStr string
	if limit > 0 {
		limitStr = fmt.Sprintf(" LIMIT %d", limit)
	}

	if device == protocol.LocalDeviceID {
		// Select all the non-ignored files with the global and need bits set.
		vals := iterStructs[fileRow](s.sql.Queryx(`
		SELECT g.name FROM files g
		INNER JOIN folders o ON o.idx = g.folder_idx
		WHERE o.folder_id = ? AND g.local_flags & ? = 0 AND g.local_flags & ? = ?
		`+orderBy+limitStr,
			folder, protocol.FlagLocalIgnored, protocol.FlagLocalNeeded|protocol.FlagLocalGlobal, protocol.FlagLocalNeeded|protocol.FlagLocalGlobal))
		return itererr.Map(vals, func(r fileRow) string {
			return osutil.NativeFilename(r.Name)
		})
	}

	// Select:
	//
	// - all the valid, non-deleted global files that don't have a corresponding
	//   remote file with the same version.
	//
	// - all the valid, deleted global files that have a corresponding non-deleted
	//   remote file (of any version)

	vals := iterStructs[fileRow](s.sql.Queryx(`
	SELECT g.name FROM files g
	INNER JOIN folders o ON o.idx = g.folder_idx
	WHERE o.folder_id = ? AND g.local_flags & ? != 0 AND NOT g.deleted AND NOT g.invalid AND NOT EXISTS (
		SELECT 1 FROM FILES f
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE f.name = g.name AND f.version = g.version AND f.folder_idx = g.folder_idx AND d.device_id = ?
	)

	UNION

	SELECT g.name FROM files g
	INNER JOIN folders o ON o.idx = g.folder_idx
	WHERE o.folder_id = ? AND g.local_flags & ? != 0 AND g.deleted AND NOT g.invalid AND EXISTS (
		SELECT 1 FROM FILES f
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE f.name = g.name AND f.folder_idx = g.folder_idx AND d.device_id = ? AND NOT f.deleted
	)
	`+orderBy+limitStr,
		folder, protocol.FlagLocalGlobal, device.String(),
		folder, protocol.FlagLocalGlobal, device.String(),
	))
	return itererr.Map(vals, func(r fileRow) string {
		return osutil.NativeFilename(r.Name)
	})
}

func (s *DB) Availability(folder, file string) ([]protocol.DeviceID, error) {
	file = osutil.NormalizedFilename(file)

	var devStrs []string
	err := s.sql.Select(&devStrs, `
		SELECT d.device_id FROM files f
		INNER JOIN devices d ON d.idx = f.device_idx
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN files g ON f.folder_idx = g.folder_idx AND g.version = f.version AND g.name = f.name
		WHERE o.folder_id = ? AND g.name = ? AND g.local_flags & ? != 0 AND f.device_idx != ?
		ORDER BY d.device_id`,
		folder, file, protocol.FlagLocalGlobal, s.localDeviceIdx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrap("availability", err)
	}

	devs := make([]protocol.DeviceID, 0, len(devStrs))
	for _, s := range devStrs {
		d, err := protocol.DeviceIDFromString(s)
		if err != nil {
			return nil, err
		}
		devs = append(devs, d)
	}

	return devs, nil
}

func (s *DB) recalcGlobalForFolder(txp *txPreparedStmts, folderIdx int64) error {
	namesStmt, err := txp.Preparex(`
	SELECT name FROM files
	WHERE folder_idx = ?
	GROUP BY name`)
	if err != nil {
		return wrap("recalc global for folder", err)
	}
	rows, err := namesStmt.Queryx(folderIdx)
	if err != nil {
		return wrap("recalc global for folder", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return wrap("recalc global for folder", err)
		}
		if err := s.recalcGlobalForFile(txp, folderIdx, name); err != nil {
			return wrap("recalc global for folder", err)
		}
	}
	return wrap("recalc global for folder", rows.Err())
}

func (s *DB) recalcGlobalForFile(txp *txPreparedStmts, folderIdx int64, file string) error {
	selStmt, err := txp.Preparex(`
		SELECT name, folder_idx, device_idx, sequence, modified, version, deleted, invalid, local_flags FROM files
		WHERE folder_idx = ? AND name = ?`)
	if err != nil {
		return wrap("processNeed (select)", err)
	}
	vals := iterStructs[fileRow](selStmt.Queryx(folderIdx, file))
	es, err := itererr.Collect(vals)
	if err != nil {
		return wrap("processNeed (select)", err)
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
	upStmt, err := txp.Prepare(`
		UPDATE files SET local_flags = ?
		WHERE folder_idx = ? AND device_idx = ? AND sequence = ?`)
	if err != nil {
		return wrap("processNeed (insert global)", err)
	}
	if _, err := upStmt.Exec(global.LocalFlags, global.FolderIdx, global.DeviceIdx, global.Sequence); err != nil {
		return wrap("processNeed (insert global)", err)
	}

	// Clear the need and global flags on all other entries
	upStmt, err = txp.Prepare(`
			UPDATE files SET local_flags = local_flags & ?
			WHERE folder_idx = ? AND name = ? AND sequence != ?`)
	if err != nil {
		return wrap("processNeed (clear need)", err)
	}
	if _, err := upStmt.Exec(^(protocol.FlagLocalNeeded | protocol.FlagLocalGlobal), folderIdx, global.Name, global.Sequence); err != nil {
		return wrap("processNeed (clear need)", err)
	}

	return nil
}

type sizesRow struct {
	Type    protocol.FileInfoType
	Count   int
	Size    int64
	FlagBit int64 `db:"local_flags"`
}

func (s *DB) LocalSize(folder string, device protocol.DeviceID) (db.Counts, error) {
	var res []sizesRow
	extra := ""
	if device == protocol.LocalDeviceID {
		// The size counters for the local device are special, in that we
		// synthetise entries with both the Global and Need flag for files
		// that we don't currently have. We need to exlude those from the
		// local size sum.
		extra = fmt.Sprintf(" AND local_flags & %[1]d != %[1]d", protocol.FlagLocalGlobal|protocol.FlagLocalNeeded)
	}
	if err := s.sql.Select(&res, `
		SELECT s.type, s.count, s.size, s.local_flags FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		INNER JOIN devices d ON d.idx = s.device_idx
		WHERE o.folder_id = ? AND d.device_id = ? AND s.local_flags & ? = 0`+extra,
		folder, device.String(), protocol.FlagLocalIgnored); err != nil {
		return db.Counts{}, err
	}
	return summarizeRows(res), nil
}

func (s *DB) NeedSize(folder string, device protocol.DeviceID) (db.Counts, error) {
	if device == protocol.LocalDeviceID {
		return s.needSizeLocal(folder)
	}
	return s.needSizeRemote(folder, device)
}

func (s *DB) needSizeLocal(folder string) (db.Counts, error) {
	// The need size for the local device is the sum of entries with both
	// the global and need bit set.
	var res []sizesRow
	err := s.sql.Select(&res, `
		SELECT s.type, s.count, s.size, s.local_flags FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		WHERE o.folder_id = ? AND s.local_flags & ? = ?
	`, folder, protocol.FlagLocalNeeded|protocol.FlagLocalGlobal, protocol.FlagLocalNeeded|protocol.FlagLocalGlobal)
	if err != nil {
		return db.Counts{}, err
	}
	return summarizeRows(res), nil
}

func (s *DB) needSizeRemote(folder string, device protocol.DeviceID) (db.Counts, error) {
	var res []sizesRow
	// See AllNeededNames for commentary as that is the same query without summing
	if err := s.sql.Select(&res, `
	SELECT g.type, count(*) as count, sum(g.size) as size, g.local_flags FROM files g
	INNER JOIN folders o ON o.idx = g.folder_idx
	WHERE o.folder_id = ? AND g.local_flags & ? != 0 AND NOT g.deleted AND NOT g.invalid AND NOT EXISTS (
		SELECT 1 FROM FILES f
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE f.name = g.name AND f.version = g.version AND f.folder_idx = g.folder_idx AND d.device_id = ?
	)
	GROUP BY g.type, g.local_flags

	UNION

	SELECT g.type, count(*) as count, sum(g.size) as size, g.local_flags FROM files g
	INNER JOIN folders o ON o.idx = g.folder_idx
	WHERE o.folder_id = ? AND g.local_flags & ? != 0 AND g.deleted AND NOT g.invalid AND EXISTS (
		SELECT 1 FROM FILES f
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE f.name = g.name AND f.folder_idx = g.folder_idx AND d.device_id = ? AND NOT f.deleted
	)
	GROUP BY g.type, g.local_flags`,
		folder, protocol.FlagLocalGlobal, device.String(),
		folder, protocol.FlagLocalGlobal, device.String()); err != nil {
		return db.Counts{}, err
	}

	return summarizeRows(res), nil
}

func (s *DB) GlobalSize(folder string) (db.Counts, error) {
	// Exclude ignored and receive-only changed files from the global count
	// (legacy expectation? it's a bit weird since those files can in fact
	// be global and you can get them with GetGlobal etc.)
	var res []sizesRow
	err := s.sql.Select(&res, `
		SELECT s.type, s.count, s.size, s.local_flags FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		WHERE o.folder_id = ? AND s.local_flags & ? != 0 AND s.local_flags & ? = 0
	`, folder, protocol.FlagLocalGlobal, protocol.FlagLocalReceiveOnly|protocol.FlagLocalIgnored)
	if err != nil {
		return db.Counts{}, err
	}
	return summarizeRows(res), nil
}

func (s *DB) ReceiveOnlySize(folder string) (db.Counts, error) {
	var res []sizesRow
	err := s.sql.Select(&res, `
		SELECT s.type, s.count, s.size, s.local_flags FROM sizes s
		INNER JOIN folders o ON o.idx = s.folder_idx
		WHERE o.folder_id = ? AND local_flags & ? != 0
	`, folder, protocol.FlagLocalReceiveOnly)
	if err != nil {
		return db.Counts{}, err
	}
	return summarizeRows(res), nil
}

func summarizeRows(res []sizesRow) db.Counts {
	c := db.Counts{
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

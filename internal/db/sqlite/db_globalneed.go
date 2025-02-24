package sqlite

import (
	"cmp"
	"database/sql"
	"errors"
	"fmt"
	"iter"
	"slices"

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
	if device != protocol.LocalDeviceID {
		return func(yield func(string, error) bool) {
			yield("", errors.New("only implemented for local device"))
		}
	}

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

	// Select all the files for the global device where the need bit is set.
	vals := iterStructs[fileRow](s.sql.Queryx(`
		SELECT g.name, g.modified, g.size FROM files g
		INNER JOIN folders o ON o.idx = g.folder_idx
		WHERE o.folder_id = ? AND g.local_flags & ? = 0 AND g.local_flags & ? = ?
		`+orderBy+limitStr,
		folder, protocol.FlagLocalIgnored, protocol.FlagLocalNeeded|protocol.FlagLocalGlobal, protocol.FlagLocalNeeded|protocol.FlagLocalGlobal))
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
	// file (we might be further down the list due to invalid flags)...
	localIdx := slices.IndexFunc(es, func(e fileRow) bool { return e.DeviceIdx == s.localDeviceIdx })
	hasLocal := localIdx >= 0 && (localIdx <= globIdx || es[localIdx].Version.Equal(global.Version.Vector))

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

	// Clear the need and global flags on non-global entries that have the
	// same version vector or are newer than the global
	for _, f := range es[:globIdx] {
		f.LocalFlags &= ^(protocol.FlagLocalNeeded | protocol.FlagLocalGlobal)
		upStmt, err := txp.Prepare(`
			UPDATE files SET local_flags = ?
			WHERE folder_idx = ? AND device_idx = ? AND sequence = ?`)
		if err != nil {
			return wrap("processNeed (clear need)", err)
		}
		if _, err := upStmt.Exec(f.LocalFlags, f.FolderIdx, f.DeviceIdx, f.Sequence); err != nil {
			return wrap("processNeed (clear need)", err)
		}
	}

	// Set the need flag and clear the global flag on all other entries
	// (these are now on the need list)
	for _, f := range es[globIdx+1:] {
		f.LocalFlags &= ^protocol.FlagLocalGlobal
		f.LocalFlags |= protocol.FlagLocalNeeded
		upStmt, err := txp.Prepare(`
			UPDATE files SET local_flags = ?
			WHERE folder_idx = ? AND device_idx = ? AND sequence = ?`)
		if err != nil {
			return wrap("processNeed (clear need)", err)
		}
		if _, err := upStmt.Exec(f.LocalFlags, f.FolderIdx, f.DeviceIdx, f.Sequence); err != nil {
			return wrap("processNeed (set need)", err)
		}
	}

	return nil
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

package sqlite

import (
	"cmp"
	"database/sql"
	"errors"
	"fmt"
	"iter"
	"slices"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
)

type fileRow struct {
	Name      string
	FolderIdx int64 `db:"folder_idx"`
	DeviceIdx int64 `db:"device_idx"`
	Sequence  int64
	Modified  int64
	Size      int64
	Version   dbVector
	Deleted   bool
	Invalid   bool
}

func (db *DB) AllNeededNames(folder string, device protocol.DeviceID, order config.PullOrder, limit int) iter.Seq2[string, error] {
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
	vals := iterStructs[fileRow](db.sql.Queryx(`
		SELECT g.name, g.modified, g.size FROM files g
		INNER JOIN folders o ON o.idx = g.folder_idx
		WHERE o.folder_id = ? AND g.device_idx = ? AND g.local_flags & ? != 0
		`+orderBy+limitStr,
		folder, db.globalDeviceIdx, protocol.FlagLocalNeeded))
	return iterMap(vals, func(r fileRow) string {
		return r.Name
	})
}

func (db *DB) Availability(folder, file string) ([]protocol.DeviceID, error) {
	file = osutil.NormalizedFilename(file)

	var devStrs []string
	err := db.sql.Select(&devStrs, `
		SELECT d.device_id FROM files f
		INNER JOIN devices d ON d.idx = f.device_idx
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN files g ON f.folder_idx = g.folder_idx AND g.version = f.version AND g.name = f.name
		WHERE o.folder_id = ? AND g.device_idx = ? AND g.name = ? AND f.device_idx != ? AND f.device_idx != ?
		ORDER BY d.device_id`,
		folder, db.globalDeviceIdx, file, db.localDeviceIdx, db.globalDeviceIdx)
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

func (db *DB) processNeed(tx *sqlx.Tx, folderIdx int64, file string) error {
	vals := iterStructs[fileRow](tx.Queryx(`
		SELECT name, folder_idx, device_idx, sequence, modified, version, deleted, invalid FROM files
		WHERE folder_idx = ? AND name = ?`,
		folderIdx, file))
	es, err := iterCollect(vals)
	if err != nil {
		return wrap("processNeed (select)", err)
	}
	if len(es) == 0 {
		// shouldn't happen
		return nil
	}

	// Sort the entries; the global entry is at the head of the list
	slices.SortFunc(es, fileRow.Compare)

	global := es[0]
	hasLocal := slices.ContainsFunc(es, func(e fileRow) bool {
		return e.DeviceIdx == db.localDeviceIdx && e.Version.Vector.Equal(global.Version.Vector)
	})

	// Set the global entry as the one with the GlobalDeviceID. Set the need
	// flag if the local device needs this file.
	flags := 0
	if !hasLocal {
		flags = protocol.FlagLocalNeeded
	}
	if _, err := tx.Exec(`
		INSERT OR REPLACE INTO files (folder_idx, device_idx, name, type, modified, size, version, deleted, invalid, local_flags, fileinfo_protobuf)
		SELECT folder_idx, ?, name, type, modified, size, version, deleted, invalid, local_flags & ? | ?, fileinfo_protobuf FROM FILES
		WHERE folder_idx = ? AND device_idx = ? AND sequence = ?`,
		db.globalDeviceIdx, ^flags, flags, global.FolderIdx, global.DeviceIdx, global.Sequence); err != nil {
		return wrap("processNeed (insert global)", err)
	}

	// Clear the need flag on non-global entries that have the same version vector
	if _, err := tx.Exec(`
		UPDATE files SET local_flags = local_flags & ?
		WHERE folder_idx = ? AND name = ? AND version = ? AND device_idx != ?`,
		^protocol.FlagLocalNeeded, global.FolderIdx, global.Name, global.Version, db.globalDeviceIdx); err != nil {
		return wrap("processNeed (clear need)", err)
	}

	// Set the need flag on all other entries (these are now on the need list)
	if _, err := tx.Exec(`
		UPDATE files SET local_flags = local_flags | ?
		WHERE folder_idx = ? AND name = ? AND version != ?`,
		protocol.FlagLocalNeeded, global.FolderIdx, global.Name, global.Version); err != nil {
		return wrap("processNeed (set need)", err)
	}

	// XXX: Files that are missing on remote devices are not represented at this point.

	return nil
}

func (e fileRow) Compare(other fileRow) int {
	// From FileInfo.WinsConflict
	vc := e.Version.Vector.Compare(other.Version.Vector)
	switch vc {
	case protocol.Equal:
		return 0
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

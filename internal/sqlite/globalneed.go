package sqlite

import (
	"cmp"
	"iter"
	"slices"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/lib/protocol"
)

type fileRow struct {
	Name      string
	FolderIdx int64 `db:"folder_idx"`
	DeviceIdx int64 `db:"device_idx"`
	Sequence  int64
	Modified  int64
	Version   dbVector
	Deleted   bool
	Invalid   bool
}

func (db *DB) AllNeededNames(folder string, device protocol.DeviceID) iter.Seq2[string, error] {
	vals := iterStructs[fileRow](db.sql.Queryx(`
		SELECT f.name, f.folder_idx, f.device_idx, f.sequence, f.modified, f.version, f.deleted, f.invalid FROM files f
		INNER JOIN folders o ON o.idx = f.folder_idx
		INNER JOIN devices d ON d.idx = f.device_idx
		WHERE o.folder_id = ? AND d.device_id = ? AND f.local_flags & ? != 0
		ORDER BY name ASC
		`,
		folder, device.String(), flagNeed))
	return iterMap(vals, func(r fileRow) string { return r.Name })
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

	// Sort the entries; the global entry is at the head of the list
	slices.SortFunc(es, fileRow.Compare)

	// Set the global entry as the one with the GlobalDeviceID
	g := es[0]
	if _, err := tx.Exec(`
		INSERT OR REPLACE INTO files (folder_idx, device_idx, name, type, modified, size, version, deleted, invalid, local_flags, fileinfo_protobuf)
		SELECT folder_idx, ?, name, type, modified, size, version, deleted, invalid, local_flags & ?, fileinfo_protobuf FROM FILES
		WHERE folder_idx = ? AND device_idx = ? AND sequence = ?`,
		db.globalDeviceIdx, ^flagNeed, g.FolderIdx, g.DeviceIdx, g.Sequence); err != nil {
		return wrap("processNeed (insert global)", err)
	}

	if hasLocalEntry := slices.ContainsFunc(es, func(e fileRow) bool { return e.DeviceIdx == db.localDeviceIdx }); !hasLocalEntry {
		// Materialize a need file (need=true, invalid=true) for the
		// local device so we can iterate them
		if _, err := tx.Exec(`
		INSERT OR REPLACE INTO files (folder_idx, device_idx, name, type, modified, size, version, deleted, invalid, local_flags, fileinfo_protobuf)
		SELECT folder_idx, ?, name, type, modified, size, "", deleted, invalid, ?, fileinfo_protobuf FROM FILES
		WHERE folder_idx = ? AND device_idx = ? AND sequence = ?`,
			db.localDeviceIdx, flagNeed, g.FolderIdx, g.DeviceIdx, g.Sequence); err != nil {
			return wrap("processNeed (insert local)", err)
		}
	}

	// Clear the need flag on the other entries that have the same version vector
	if _, err := tx.Exec(`
		UPDATE files SET local_flags = local_flags & ?
		WHERE folder_idx = ? AND name = ? AND version = ?`,
		^flagNeed, g.FolderIdx, g.Name, g.Version); err != nil {
		return wrap("processNeed (clear need)", err)
	}

	// Set the need flag on all other entries (these are now on the need list)
	if _, err := tx.Exec(`
		UPDATE files SET local_flags = local_flags | ?
		WHERE folder_idx = ? AND name = ? AND version != ?`,
		flagNeed, g.FolderIdx, g.Name, g.Version); err != nil {
		return wrap("processNeed (set need)", err)
	}

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

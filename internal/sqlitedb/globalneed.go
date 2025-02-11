package sqlitedb

import (
	"cmp"
	"slices"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/lib/protocol"
)

type globalEntry struct {
	Name      string
	FolderIdx int64 `db:"folder_idx"`
	DeviceIdx int64 `db:"device_idx"`
	Sequence  int64
	Modified  int64
	Version   dbVector
	Deleted   bool
	Invalid   bool
}

func (db *DB) processNeed(tx *sqlx.Tx, folder, file string) error {
	vals := iterStructs[globalEntry](tx.Queryx(`
		SELECT f.name, f.folder_idx, f.device_idx, f.sequence, f.modified, f.version, f.deleted FROM files f
		INNER JOIN folders o ON f.folder_idx = o.idx
		WHERE f.name = $1 AND o.folder_id = $2`,
		file, folder))
	es, err := iterCollect(vals)
	if err != nil {
		return err
	}
	return db.processNeedSet(tx, es)
}

func (db *DB) processNeedSet(tx *sqlx.Tx, es []globalEntry) error {
	// Sort the entries; the global entry is at the head of the list
	slices.SortFunc(es, globalEntry.Compare)

	// We will maintain one entry for each device (XXX: that shares the folder, ideally)
	var deviceIdxs []int
	if err := tx.Select(&deviceIdxs, `SELECT idx FROM devices`); err != nil {
		return wrap("processNeed", err)
	}
	seenDeviceIdxs := make(map[int]struct{})

	for i, e := range es {
		switch {
		case i == 0:
			if _, err := tx.Exec(`
				INSERT OR REPLACE INTO globals (folder_idx, device_idx, file_sequence, name)
				VALUES ($1, $2, $3, $4)`,
				e.FolderIdx, e.DeviceIdx, e.Sequence, e.Name); err != nil {
				return wrap("processNeedSet", err)
			}
			fallthrough

		case e.Version.Equal(es[0].Version.Vector):
			// The global entry is never needed, nor others that are identical to it
			if _, err := tx.Exec(`DELETE FROM needs WHERE folder_idx = $1 AND device_idx = $2`, e.FolderIdx, e.DeviceIdx); err != nil {
				return wrap("processNeedSet", err)
			}
			seenDeviceIdxs[int(e.DeviceIdx)] = struct{}{}

		default:
			// Need it
			if _, err := tx.Exec(`INSERT OR IGNORE INTO needs (folder_idx, device_idx, file_sequence, name) VALUES ($1, $2, $3, $4)`, e.FolderIdx, e.DeviceIdx, e.Sequence, e.Name); err != nil {
				return wrap("processNeedSet", err)
			}
			seenDeviceIdxs[int(e.DeviceIdx)] = struct{}{}
		}
	}

	global := es[0]
	for _, idx := range deviceIdxs {
		if _, seen := seenDeviceIdxs[idx]; seen {
			continue
		}
		// Need it
		if _, err := tx.Exec(`INSERT OR IGNORE INTO needs (folder_idx, device_idx, file_sequence, name) VALUES ($1, $2, null, $3)`, global.FolderIdx, idx, global.Name); err != nil {
			return wrap("processNeedSet", err)
		}
	}
	return nil
}

func (e globalEntry) Compare(other globalEntry) int {
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

package sqlite

import (
	"cmp"
	"slices"
	"sync/atomic"
	"time"

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
		SELECT f.name, f.folder_idx, f.device_idx, f.sequence, f.modified, f.version, f.deleted, f.invalid FROM files f
		INNER JOIN folders o ON f.folder_idx = o.idx
		WHERE f.name = $1 AND o.folder_id = $2`,
		file, folder))
	es, err := iterCollect(vals)
	if err != nil {
		return err
	}

	// Sort the entries; the global entry is at the head of the list
	slices.SortFunc(es, globalEntry.Compare)

	g := es[0]
	if _, err := tx.Exec(`
		INSERT OR REPLACE INTO files (folder_idx, device_idx, sequence, name, type, modified, size, version, deleted, invalid, local_flags, fileinfo_protobuf)
		SELECT folder_idx, $1, $2, name, type, modified, size, version, deleted, invalid, local_flags | $3, fileinfo_protobuf FROM FILES
		WHERE folder_idx = $4 AND device_idx = $5 AND sequence = $6`,
		db.globalDeviceIdx, monotonicNano(), flagInSync, g.FolderIdx, g.DeviceIdx, g.Sequence); err != nil {
		return wrap("processNeed", err)
	}
	if _, err := tx.Exec(`
		UPDATE files SET local_flags = local_flags | ?
		WHERE folder_idx = ? AND name = ? AND version = ?`,
		flagInSync, g.FolderIdx, g.Name, g.Version); err != nil {
		return wrap("processNeed", err)
	}
	if _, err := tx.Exec(`
		UPDATE files SET local_flags = local_flags & ?
		WHERE folder_idx = ? AND name = ? AND version != ?`,
		^flagInSync, g.FolderIdx, g.Name, g.Version); err != nil {
		return wrap("processNeed", err)
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

var lastNano atomic.Int64

func monotonicNano() int64 {
	t := time.Now().UnixNano()
	for {
		p := lastNano.Load()
		if t <= p {
			t = p + 1
		}
		if lastNano.CompareAndSwap(p, t) {
			return t
		}
	}
}

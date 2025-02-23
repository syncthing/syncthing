package sqlite

import (
	"encoding/hex"
	"iter"

	"github.com/jmoiron/sqlx"
	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
)

type BlockMapEntry struct {
	FolderID string `db:"folder_id"`
	Name     string
	Index    int `db:"idx"`
	Offset   int64
	Size     int
}

func (*DB) insertBlocksLocked(tx *sqlx.Tx, folderIdx, deviceIdx, localSeq int64, blocks []protocol.BlockInfo) error {
	for i, b := range blocks {
		if _, err := tx.Exec(`
			INSERT OR REPLACE INTO blocks (hash, folder_idx, device_idx, file_sequence, idx, offset, size)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			hex.EncodeToString(b.Hash), folderIdx, deviceIdx, localSeq, i, b.Offset, b.Size); err != nil {
			return wrap("insert block", err)
		}
	}
	return nil
}

func (db *DB) Blocks(hash []byte) iter.Seq2[BlockMapEntry, error] {
	vals := iterStructs[BlockMapEntry](db.sql.Queryx(`
		SELECT o.folder_id, f.name, b.idx, b.offset, b.size FROM blocks b
		INNER JOIN files f ON f.sequence = b.file_sequence
		INNER JOIN folders o ON b.folder_idx = o.idx
		WHERE b.hash = ? AND b.device_idx = ?
		ORDER BY o.folder_id, f.name, b.idx`,
		hex.EncodeToString(hash), db.localDeviceIdx))
	return itererr.Map(vals, func(v BlockMapEntry) BlockMapEntry {
		v.Name = osutil.NativeFilename(v.Name)
		return v
	})
}

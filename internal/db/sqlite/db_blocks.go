package sqlite

import (
	"iter"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/lib/protocol"
)

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
	if _, err := tx.NamedExec(`
		INSERT OR IGNORE INTO blocks (hash, blocklist_hash, idx, offset, size)
		VALUES (:hash, :blocklist_hash, :idx, :offset, :size)`, bs); err != nil {
		return wrap("insert block", err)
	}
	return nil
}

func (s *DB) Blocks(hash []byte) iter.Seq2[db.BlockMapEntry, error] {
	return iterStructs[db.BlockMapEntry](s.sql.Queryx(`
		SELECT b.blocklist_hash, b.idx, b.offset, b.size FROM blocks b
		WHERE b.hash = ?`,
		hash))
}

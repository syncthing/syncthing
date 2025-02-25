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
	// We involve the files table in this select because deletion of blocks
	// & blocklists is deferred (gabrage collected) while the files list is
	// not. This filters out blocks that are in fact deleted.
	return iterStructs[db.BlockMapEntry](s.sql.Queryx(`
		SELECT f.blocklist_hash, b.idx, b.offset, b.size FROM files f
		LEFT JOIN blocks b ON f.blocklist_hash = b.blocklist_hash
		WHERE b.hash = ?`,
		hash))
}

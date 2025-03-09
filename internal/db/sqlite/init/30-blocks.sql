-- Blocks
--
-- For all local files we store the blocks individually for quick lookup. A
-- given block can exist in multiple blocklists and at multiple offsets in a
-- blocklist.
CREATE TABLE IF NOT EXISTS blocks (
    hash 			BLOB NOT NULL,
    blocklist_hash 	BLOB NOT NULL,
    idx             INTEGER NOT NULL,
    offset          INTEGER NOT NULL,
    size            INTEGER NOT NULL,
    FOREIGN KEY(blocklist_hash) REFERENCES blocklists(blocklist_hash) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED
) STRICT
;
CREATE UNIQUE INDEX IF NOT EXISTS blocks_hash_position ON blocks (hash, blocklist_hash, idx)
;

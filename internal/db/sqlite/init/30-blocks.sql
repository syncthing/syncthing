-- Block lists
--
-- The block lists are extracted from FileInfos and stored separately. This
-- reduces the database size by reusing the same block list entry for all
-- devices announcing the same file. Doing it for all block lists instead of
-- using a size cutoff simplifies queries. Block lists are garbage collected
-- "manually", not using a trigger as that was too performance impacting.
CREATE TABLE IF NOT EXISTS blocklists (
    blocklist_hash BLOB NOT NULL PRIMARY KEY,
    blprotobuf BLOB NOT NULL
) STRICT
;

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

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

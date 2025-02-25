-- Block lists
--
-- The blocklists are extracted from FileInfos and stored separately when
-- they're over a certain size. This reduces the database size by reusing
-- the same blocklist entry for all devices announcing the same file.
-- Blocklists are garbage collected "manually", not using a trigger as that
-- was too performance impacting.
CREATE TABLE IF NOT EXISTS blocklists (
    blocklist_hash BLOB NOT NULL PRIMARY KEY,
    blprotobuf BLOB NOT NULL
) STRICT
;

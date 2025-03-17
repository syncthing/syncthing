--- Simple KV store. This backs the "miscDB" we use for certain minor pieces
--  of data.
CREATE TABLE IF NOT EXISTS kv (
    key TEXT NOT NULL PRIMARY KEY COLLATE BINARY,
    value BLOB NOT NULL
) STRICT
;

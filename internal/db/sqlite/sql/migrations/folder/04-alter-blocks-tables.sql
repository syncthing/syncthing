-- Copyright (C) 2025 The Syncthing Authors.
--
-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this file,
-- You can obtain one at https://mozilla.org/MPL/2.0/.

-- Copy blocks to new table with fewer indexes

DROP TABLE IF EXISTS blocks_v4
;

CREATE TABLE blocks_v4 (
    hash BLOB NOT NULL,
    blocklist_hash BLOB NOT NULL,
    idx INTEGER NOT NULL,
    offset INTEGER NOT NULL,
    size INTEGER NOT NULL,
    PRIMARY KEY (hash, blocklist_hash, idx)
) STRICT, WITHOUT ROWID
;

INSERT INTO blocks_v4 (hash, blocklist_hash, idx, offset, size)
SELECT hash, blocklist_hash, idx, offset, size FROM blocks ORDER BY hash, blocklist_hash, idx
;

DROP TABLE blocks
;

ALTER TABLE blocks_v4 RENAME TO blocks
;

-- Copy blocklists to new table with fewer indexes

DROP TABLE IF EXISTS blocklists_v4
;

CREATE TABLE blocklists_v4 (
    blocklist_hash BLOB NOT NULL PRIMARY KEY,
    blprotobuf BLOB NOT NULL
) STRICT, WITHOUT ROWID
;

INSERT INTO blocklists_v4 (blocklist_hash, blprotobuf)
SELECT blocklist_hash, blprotobuf FROM blocklists ORDER BY blocklist_hash
;

DROP TABLE blocklists
;

ALTER TABLE blocklists_v4 RENAME TO blocklists
;

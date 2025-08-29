-- Copyright (C) 2025 The Syncthing Authors.
--
-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this file,
-- You can obtain one at https://mozilla.org/MPL/2.0/.

-- Block lists
--
-- The block lists are extracted from FileInfos and stored separately. This
-- reduces the database size by reusing the same block list entry for all
-- devices announcing the same file. Doing it for all block lists instead of
-- using a size cutoff simplifies queries. Block lists are garbage collected
-- "manually", not using a trigger as that was too performance impacting.
CREATE TABLE IF NOT EXISTS blocklists (
    id INTEGER PRIMARY KEY,
    blocklist_hash BLOB NOT NULL,
    blprotobuf BLOB NOT NULL
) STRICT, WITHOUT ROWID
;
CREATE UNIQUE INDEX IF NOT EXISTS blockslists_unique ON blocklists (blocklist_hash)
;

-- Blocks
--
-- For all local files we store the blocks individually for quick lookup. A
-- given block can exist in multiple blocklists and at multiple offsets in a
-- blocklist.
CREATE TABLE IF NOT EXISTS blocks (
    id INTEGER PRIMARY KEY,
    hash BLOB NOT NULL,
    blocklist_hash BLOB NOT NULL,
    idx INTEGER NOT NULL,
    offset INTEGER NOT NULL,
    size INTEGER NOT NULL,
    FOREIGN KEY(blocklist_hash) REFERENCES blocklists(blocklist_hash) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED
) STRICT, WITHOUT ROWID
;
CREATE UNIQUE INDEX IF NOT EXISTS blocks_unique ON blocks (hash, blocklist_hash, idx)
;

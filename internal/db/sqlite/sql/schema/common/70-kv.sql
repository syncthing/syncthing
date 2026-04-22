-- Copyright (C) 2025 The Syncthing Authors.
--
-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this file,
-- You can obtain one at https://mozilla.org/MPL/2.0/.

--- Simple KV store. This backs the "miscDB" we use for certain minor pieces
--  of data.
CREATE TABLE IF NOT EXISTS kv (
    key TEXT NOT NULL PRIMARY KEY COLLATE BINARY,
    value BLOB NOT NULL
) STRICT, WITHOUT ROWID
;

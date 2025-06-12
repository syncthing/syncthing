-- Copyright (C) 2025 The Syncthing Authors.
--
-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this file,
-- You can obtain one at https://mozilla.org/MPL/2.0/.

-- folders map folder IDs as used by Syncthing to database folder indexes
CREATE TABLE IF NOT EXISTS folders (
    idx INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    folder_id TEXT NOT NULL UNIQUE COLLATE BINARY,
    database_name TEXT COLLATE BINARY -- initially null
) STRICT
;
-- The database_name is unique, when set
CREATE INDEX IF NOT EXISTS folders_database_name ON folders (database_name) WHERE database_name IS NOT NULL
;

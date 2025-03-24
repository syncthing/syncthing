-- Copyright (C) 2025 The Syncthing Authors.
--
-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this file,
-- You can obtain one at https://mozilla.org/MPL/2.0/.

-- folders map folder IDs as used by Syncthing to database folder indexes
CREATE TABLE IF NOT EXISTS folders (
    idx INTEGER NOT NULL PRIMARY KEY,
    folder_id TEXT NOT NULL UNIQUE COLLATE BINARY
) STRICT
;

-- devices map device IDs as used by Syncthing to database device indexes
CREATE TABLE IF NOT EXISTS devices (
    idx INTEGER NOT NULL PRIMARY KEY,
    device_id TEXT NOT NULL UNIQUE COLLATE BINARY
) STRICT
;

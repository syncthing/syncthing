-- Copyright (C) 2025 The Syncthing Authors.
--
-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this file,
-- You can obtain one at https://mozilla.org/MPL/2.0/.

-- Schema migrations hold the list of historical migrations applied
CREATE TABLE IF NOT EXISTS schemamigrations (
    schema_version INTEGER NOT NULL PRIMARY KEY,
    applied_at INTEGER NOT NULL, -- unix nanos
    syncthing_version TEXT NOT NULL COLLATE BINARY
) STRICT
;

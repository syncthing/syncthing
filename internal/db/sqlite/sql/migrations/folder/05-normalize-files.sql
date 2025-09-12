-- Copyright (C) 2025 The Syncthing Authors.
--
-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this file,
-- You can obtain one at https://mozilla.org/MPL/2.0/.

-- Grab all unique names into the names table

INSERT INTO file_names (idx, name) SELECT DISTINCT null, name FROM files
;

-- Grab all unique versions into the versions table

INSERT INTO file_versions (idx, version) SELECT DISTINCT null, version FROM files
;

-- Create the new files table

DROP TABLE IF EXISTS files_v5
;

CREATE TABLE files_v5 (
    device_idx INTEGER NOT NULL,
    sequence INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    remote_sequence INTEGER,
    name_idx INTEGER NOT NULL, -- changed
    type INTEGER NOT NULL,
    modified INTEGER NOT NULL,
    size INTEGER NOT NULL,
    version_idx INTEGER NOT NULL, -- changed
    deleted INTEGER NOT NULL,
    local_flags INTEGER NOT NULL,
    blocklist_hash BLOB,
    FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
    FOREIGN KEY(name_idx) REFERENCES file_names(idx), -- added
    FOREIGN KEY(version_idx) REFERENCES file_versions(idx) -- added
) STRICT
;

-- Populate the new files table and move it in place

INSERT INTO files_v5
    SELECT f.device_idx, f.sequence, f.remote_sequence, n.idx as name_idx, f.type, f.modified, f.size, v.idx as version_idx, f.deleted, f.local_flags, f.blocklist_hash
    FROM files f
    INNER JOIN file_names n ON n.name = f.name
    INNER JOIN file_versions v ON v.version = f.version
;

DROP TABLE files
;

ALTER TABLE files_v5 RENAME TO files
;

-- Copyright (C) 2025 The Syncthing Authors.
--
-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this file,
-- You can obtain one at https://mozilla.org/MPL/2.0/.

-- Files
--
-- The files table contains all files announced by any device. Files present
-- on this device are filed under the LocalDeviceID, not the actual current
-- device ID, for simplicity, consistency and portability. One announced
-- version of each file is considered the "global" version - the latest one,
-- that all other devices strive to replicate. This instance gets the Global
-- flag bit set. There may be other identical instances of this file
-- announced by other devices, but only one instance gets the Global flag;
-- this simplifies accounting. If the current device has the Global version,
-- the LocalDeviceID instance of the file is the one that has the Global
-- bit.
--
-- If the current device does not have that version of the file it gets the
-- Need bit set. Only Global files announced by another device can have the
-- Need bit. This allows for very efficient lookup of files needing handling
-- on this device, which is a common query.
CREATE TABLE IF NOT EXISTS files (
    device_idx INTEGER NOT NULL, -- actual device ID or LocalDeviceID
    sequence INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT, -- our local database sequence, for each and every entry
    remote_sequence INTEGER, -- remote device's sequence number, null for local or synthetic entries
    name_idx INTEGER NOT NULL,
    type INTEGER NOT NULL, -- protocol.FileInfoType
    modified INTEGER NOT NULL, -- Unix nanos
    size INTEGER NOT NULL,
    version_idx INTEGER NOT NULL,
    deleted INTEGER NOT NULL, -- boolean
    local_flags INTEGER NOT NULL,
    blocklist_hash BLOB, -- null when there are no blocks
    FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
    FOREIGN KEY(name_idx) REFERENCES file_names(idx),
    FOREIGN KEY(version_idx) REFERENCES file_versions(idx)
) STRICT
;
CREATE TABLE IF NOT EXISTS file_names (
    idx INTEGER NOT NULL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE COLLATE BINARY
) STRICT
;
CREATE TABLE IF NOT EXISTS file_versions (
    idx INTEGER NOT NULL PRIMARY KEY,
    version TEXT NOT NULL UNIQUE COLLATE BINARY
) STRICT
;
-- FileInfos store the actual protobuf object. We do this separately to keep
-- the files rows smaller and more efficient.
CREATE TABLE IF NOT EXISTS fileinfos (
    sequence INTEGER NOT NULL PRIMARY KEY, -- our local database sequence from the files table
    fiprotobuf BLOB NOT NULL,
    FOREIGN KEY(sequence) REFERENCES files(sequence) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED
) STRICT
;
-- There can be only one file per folder, device, and remote sequence number
CREATE UNIQUE INDEX IF NOT EXISTS files_remote_sequence ON files (device_idx, remote_sequence)
    WHERE remote_sequence IS NOT NULL
;
-- There can be only one file per folder, device, and name
CREATE UNIQUE INDEX IF NOT EXISTS files_device_name ON files (device_idx, name_idx)
;
-- We want to be able to look up & iterate files based on blocks hash
CREATE INDEX IF NOT EXISTS files_blocklist_hash_only ON files (blocklist_hash, device_idx) WHERE blocklist_hash IS NOT NULL
;
-- We need to look by name_idx or version_idx for garbage collection.
-- This will fail pre-migration for v4 schemas, which is fine.
-- syncthing:ignore-failure
CREATE INDEX IF NOT EXISTS files_name_idx_only ON files (name_idx)
;
-- This will fail pre-migration for v4 schemas, which is fine.
-- syncthing:ignore-failure
CREATE INDEX IF NOT EXISTS files_version_idx_only ON files (version_idx)
;

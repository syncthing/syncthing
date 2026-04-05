-- Copyright (C) 2025 The Syncthing Authors.
--
-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this file,
-- You can obtain one at https://mozilla.org/MPL/2.0/.

-- Counts
--
-- Counts and sizes are maintained for each device, folder, type, flag bits
-- combination.
CREATE TABLE IF NOT EXISTS counts (
    device_idx INTEGER NOT NULL,
    type INTEGER NOT NULL,
    local_flags INTEGER NOT NULL,
    deleted INTEGER NOT NULL, -- boolean
    count INTEGER NOT NULL,
    size INTEGER NOT NULL,
    PRIMARY KEY(device_idx, type, local_flags, deleted),
    FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE
) STRICT, WITHOUT ROWID
;

--- Maintain counts when files are added and removed using triggers

CREATE TRIGGER IF NOT EXISTS counts_insert AFTER INSERT ON files
BEGIN
    INSERT INTO counts (device_idx, type, local_flags, deleted, count, size)
        VALUES (NEW.device_idx, NEW.type, NEW.local_flags, NEW.deleted, 1, NEW.size)
        ON CONFLICT DO UPDATE SET count = count + 1, size = size + NEW.size;
END
;
CREATE TRIGGER IF NOT EXISTS counts_delete AFTER DELETE ON files
BEGIN
    UPDATE counts SET count = count - 1, size = size - OLD.size
        WHERE device_idx = OLD.device_idx AND type = OLD.type AND local_flags = OLD.local_flags AND deleted = OLD.deleted;
END
;
CREATE TRIGGER IF NOT EXISTS counts_update AFTER UPDATE OF local_flags ON files
WHEN NEW.local_flags != OLD.local_flags
BEGIN
    INSERT INTO counts (device_idx, type, local_flags, deleted, count, size)
        VALUES (NEW.device_idx, NEW.type, NEW.local_flags, NEW.deleted, 1, NEW.size)
        ON CONFLICT DO UPDATE SET count = count + 1, size = size + NEW.size;
    UPDATE counts SET count = count - 1, size = size - OLD.size
        WHERE device_idx = OLD.device_idx AND type = OLD.type AND local_flags = OLD.local_flags AND deleted = OLD.deleted;
END
;

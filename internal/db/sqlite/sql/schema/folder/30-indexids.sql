-- Copyright (C) 2025 The Syncthing Authors.
--
-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this file,
-- You can obtain one at https://mozilla.org/MPL/2.0/.

-- indexids holds the index ID and maximum sequence for a given device and folder
CREATE TABLE IF NOT EXISTS indexids (
    device_idx INTEGER NOT NULL PRIMARY KEY,
    index_id TEXT NOT NULL COLLATE BINARY,
    sequence INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE
) STRICT, WITHOUT ROWID
;
CREATE TRIGGER IF NOT EXISTS indexids_seq AFTER INSERT ON files
BEGIN
    INSERT INTO indexids (device_idx, index_id, sequence)
        VALUES (NEW.device_idx, "", COALESCE(NEW.remote_sequence, NEW.sequence))
        ON CONFLICT DO UPDATE SET sequence = COALESCE(NEW.remote_sequence, NEW.sequence);
END
;

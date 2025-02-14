--- Files
CREATE TABLE IF NOT EXISTS files (
    folder_idx INTEGER NOT NULL,
    device_idx INTEGER NOT NULL, -- actual device ID, or LocalDeviceID, or GlobalDeviceID
    sequence INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT, -- our local database sequence, for each and every entry
    remote_sequence INTEGER, -- remote device's sequence number, null for local or synthetic entries
    name TEXT NOT NULL,
    type INTEGER NOT NULL, -- protocol.FileInfoType
    modified INTEGER NOT NULL, -- Unix nanos
    size INTEGER NOT NULL,
    version TEXT NOT NULL,
    deleted INTEGER NOT NULL, -- boolean
    invalid INTEGER NOT NULL, -- boolean
    local_flags  INTEGER NOT NULL,
    fileinfo_protobuf BLOB NOT NULL,
    FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
    FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE
) STRICT
;
-- There can be only one file per folder, device, and remote sequence number
CREATE UNIQUE INDEX IF NOT EXISTS files_remote_sequence ON files (folder_idx, device_idx, remote_sequence)
;
-- There can be only one file per folder, device, and name
CREATE UNIQUE INDEX IF NOT EXISTS files_device_name ON files (folder_idx, device_idx, name)
;
-- We want to be able to look up & iterate files based on just folder and name
CREATE INDEX IF NOT EXISTS files_name_only ON files (folder_idx, name)
;
--- Files
CREATE TABLE IF NOT EXISTS files (
    folder_idx INTEGER NOT NULL,
    device_idx INTEGER NOT NULL, -- actual device ID, or LocalDeviceID, or GlobalDeviceID
    sequence INTEGER NOT NULL,
    name TEXT NOT NULL,
    type INTEGER NOT NULL, -- protocol.FileInfoType
    modified INTEGER NOT NULL, -- Unix nanos
    size INTEGER NOT NULL,
    version TEXT NOT NULL,
    deleted INTEGER NOT NULL, -- boolean
    invalid INTEGER NOT NULL, -- boolean
    local_flags  INTEGER NOT NULL,
    fileinfo_protobuf BLOB NOT NULL,
    PRIMARY KEY(folder_idx, device_idx, sequence),
    FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
    FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE
) STRICT
;
CREATE UNIQUE INDEX IF NOT EXISTS files_device_name ON files (folder_idx, device_idx, name)
;
CREATE INDEX IF NOT EXISTS files_name_only ON files (folder_idx, name)
;

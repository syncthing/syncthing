-- folders map folder IDs as used by Syncthing to database folder indexes
CREATE TABLE IF NOT EXISTS folders (
    idx INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    folder_id TEXT NOT NULL UNIQUE
) STRICT
;

-- devices map device IDs as used by Syncthing to database device indexes
CREATE TABLE IF NOT EXISTS devices (
    idx INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL UNIQUE
) STRICT
;

-- index_ids holds the index ID for a given device and folder
CREATE TABLE IF NOT EXISTS index_ids (
    device_idx INTEGER NOT NULL,
    folder_idx INTEGER NOT NULL,
    index_id INTEGER NOT NULL,
    PRIMARY KEY(device_idx, folder_idx),
    FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE,
    FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE
) STRICT
;

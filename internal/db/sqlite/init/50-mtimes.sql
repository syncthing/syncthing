--- Backing for the MtimeFS
CREATE TABLE IF NOT EXISTS mtimes (
    folder_idx INTEGER NOT NULL,
    name TEXT NOT NULL,
    ondisk INTEGER NOT NULL, -- unix nanos
    virtual INTEGER NOT NULL, -- unix nanos
    PRIMARY KEY(folder_idx, name),
    FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE
) STRICT
;

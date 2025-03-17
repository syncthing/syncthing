-- Add the "deleted" column to counts, setting it based on the previous
-- local flag bit. Somewhat complicated because it requires changing the
-- primary key, which requires a new table, which requires juggling the
-- triggers.

DROP TRIGGER counts_insert
;
DROP TRIGGER counts_delete
;
DROP TRIGGER counts_update_add
;
DROP TRIGGER counts_update_del
;

CREATE TABLE counts_new (
    folder_idx INTEGER NOT NULL,
    device_idx INTEGER NOT NULL,
    type INTEGER NOT NULL,
    local_flags INTEGER NOT NULL,
    count INTEGER NOT NULL,
    size INTEGER NOT NULL,
    deleted INTEGER NOT NULL, -- boolean
    PRIMARY KEY(folder_idx, device_idx, type, local_flags, deleted),
    FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
    FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE
) STRICT
;
INSERT INTO counts_new (folder_idx, device_idx, type, local_flags, count, size, deleted)
    SELECT folder_idx, device_idx, type, local_flags & ~64, count, size, local_flags & 64 != 0
    FROM counts
;
DROP TABLE counts
;
ALTER TABLE counts_new RENAME TO counts
;

CREATE TRIGGER IF NOT EXISTS counts_insert AFTER INSERT ON files
BEGIN
    INSERT INTO counts (folder_idx, device_idx, type, local_flags, count, size, deleted)
        VALUES (NEW.folder_idx, NEW.device_idx, NEW.type, NEW.local_flags, 1, NEW.size, NEW.deleted)
        ON CONFLICT DO UPDATE SET count = count + 1, size = size + NEW.size;
END
;
CREATE TRIGGER IF NOT EXISTS counts_delete AFTER DELETE ON files
BEGIN
    UPDATE counts SET count = count - 1, size = size - OLD.size
        WHERE folder_idx = OLD.folder_idx AND device_idx = OLD.device_idx AND type = OLD.type AND local_flags = OLD.local_flags AND deleted = OLD.deleted;
END
;
CREATE TRIGGER IF NOT EXISTS counts_update_add AFTER UPDATE ON files
WHEN NEW.local_flags != OLD.local_flags
BEGIN
    INSERT INTO counts (folder_idx, device_idx, type, local_flags, count, size, deleted)
        VALUES (NEW.folder_idx, NEW.device_idx, NEW.type, NEW.local_flags, 1, NEW.size, NEW.deleted)
        ON CONFLICT DO UPDATE SET count = count + 1, size = size + NEW.size;
END
;
CREATE TRIGGER IF NOT EXISTS counts_update_del AFTER UPDATE ON files
WHEN NEW.local_flags != OLD.local_flags
BEGIN
    UPDATE counts SET count = count - 1, size = size - OLD.size
        WHERE folder_idx = OLD.folder_idx AND device_idx = OLD.device_idx AND type = OLD.type AND local_flags = OLD.local_flags AND deleted = OLD.deleted;
END
;


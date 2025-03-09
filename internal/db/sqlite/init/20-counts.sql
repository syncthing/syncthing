-- Counts
--
-- Counts and sizes are maintained for each device, folder, type, flag bits
-- combination.
CREATE TABLE IF NOT EXISTS counts (
    folder_idx INTEGER NOT NULL,
    device_idx INTEGER NOT NULL,
    type INTEGER NOT NULL,
    local_flags INTEGER NOT NULL,
    count INTEGER NOT NULL,
    size INTEGER NOT NULL,
    PRIMARY KEY(folder_idx, device_idx, type, local_flags),
    FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
    FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE
) STRICT
;

--- Maintain counts when files are added and removed using triggers

CREATE TRIGGER IF NOT EXISTS counts_insert AFTER INSERT ON files
BEGIN
    INSERT INTO counts (folder_idx, device_idx, type, local_flags, count, size)
        VALUES (NEW.folder_idx, NEW.device_idx, NEW.type, NEW.local_flags, 1, NEW.size)
        ON CONFLICT DO UPDATE SET count = count + 1, size = size + NEW.size;
END
;
CREATE TRIGGER IF NOT EXISTS counts_delete AFTER DELETE ON files
BEGIN
    UPDATE counts SET count = count - 1, size = size - OLD.size
        WHERE folder_idx = OLD.folder_idx AND device_idx = OLD.device_idx AND type = OLD.type AND local_flags = OLD.local_flags;
END
;
CREATE TRIGGER IF NOT EXISTS counts_update_add AFTER UPDATE ON files
WHEN NEW.local_flags != OLD.local_flags
BEGIN
    INSERT INTO counts (folder_idx, device_idx, type, local_flags, count, size)
        VALUES (NEW.folder_idx, NEW.device_idx, NEW.type, NEW.local_flags, 1, NEW.size)
        ON CONFLICT DO UPDATE SET count = count + 1, size = size + NEW.size;
END
;
CREATE TRIGGER IF NOT EXISTS counts_update_del AFTER UPDATE ON files
WHEN NEW.local_flags != OLD.local_flags
BEGIN
    UPDATE counts SET count = count - 1, size = size - OLD.size
        WHERE folder_idx = OLD.folder_idx AND device_idx = OLD.device_idx AND type = OLD.type AND local_flags = OLD.local_flags;
END
;

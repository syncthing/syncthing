--- This init script is executed at startup to set up the database.
--- Statements must be separated by a semicolon by itself on a separate line.

CREATE TABLE IF NOT EXISTS folders (
    idx INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    folder_id TEXT NOT NULL UNIQUE
) STRICT
;

CREATE TABLE IF NOT EXISTS devices (
    idx INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL UNIQUE
) STRICT
;

--- Files
CREATE TABLE IF NOT EXISTS files (
    folder_idx INTEGER NOT NULL,
    device_idx INTEGER NOT NULL,
    sequence INTEGER NOT NULL,
    name TEXT NOT NULL,
    type INTEGER NOT NULL, -- protocol.FileInfoType
    modified INTEGER NOT NULL, -- Unix nanos
    size INTEGER NOT NULL,
    version TEXT NOT NULL,
    deleted INTEGER NOT NULL, -- boolean
    invalid INTEGER NOT NULL, -- boolean
    fileinfo_protobuf BLOB NOT NULL,
    PRIMARY KEY(folder_idx, device_idx, sequence),
    FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
    FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE
) STRICT
;
CREATE UNIQUE INDEX IF NOT EXISTS files_name ON files (folder_idx, device_idx, name)
;

--- Maintain size counts when files are added and removed
--- Files are never updated, only replaced, so we handle inserts and deletes
CREATE TABLE IF NOT EXISTS sizes (
    folder_idx INTEGER NOT NULL,
    device_idx INTEGER NOT NULL,
    files INTEGER NOT NULL,
    directories INTEGER NOT NULL,
    symlinks INTEGER NOT NULL,
    total_size INTEGER NOT NULL,
    PRIMARY KEY(folder_idx, device_idx),
    FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
    FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE
) STRICT
;
CREATE TRIGGER IF NOT EXISTS sizes_insert_file AFTER INSERT ON files
WHEN NOT NEW.invalid AND NOT NEW.deleted AND NEW.type = 0 -- FileInfoTypeFile
BEGIN
    INSERT INTO sizes (folder_idx, device_idx, files, directories, symlinks, total_size)
        VALUES (NEW.folder_idx, NEW.device_idx, 1, 0, 0, NEW.size)
        ON CONFLICT DO UPDATE SET files = files + 1, total_size = total_size + NEW.size;
END
;
CREATE TRIGGER IF NOT EXISTS sizes_insert_dir AFTER INSERT ON files
WHEN NOT NEW.invalid AND NOT NEW.deleted AND NEW.type = 1 -- FileInfoTypeDirectory
BEGIN
    INSERT INTO sizes (folder_idx, device_idx, files, directories, symlinks, total_size)
        VALUES (NEW.folder_idx, NEW.device_idx, 0, 1, 0, NEW.size)
        ON CONFLICT DO UPDATE SET directories = directories + 1, total_size = total_size + NEW.size;
END
;
CREATE TRIGGER IF NOT EXISTS sizes_insert_symlink AFTER INSERT ON files
WHEN NOT NEW.invalid AND NOT NEW.deleted AND NEW.type = 4 -- FileInfoTypeSymlink
BEGIN
    INSERT INTO sizes (folder_idx, device_idx, files, directories, symlinks, total_size)
        VALUES (NEW.folder_idx, NEW.device_idx, 0, 0, 1, NEW.size)
        ON CONFLICT DO UPDATE SET symlinks = symlinks + 1, total_size = total_size + NEW.size;
END
;
CREATE TRIGGER IF NOT EXISTS sizes_delete_file AFTER DELETE ON files
WHEN NOT OLD.invalid AND NOT OLD.deleted AND OLD.type = 0 -- FileInfoTypeFile
BEGIN
    UPDATE sizes SET files = files - 1, total_size = total_size - OLD.size
        WHERE folder_idx = OLD.folder_idx AND device_idx = OLD.device_idx;
END
;
CREATE TRIGGER IF NOT EXISTS sizes_delete_dir AFTER DELETE ON files
WHEN NOT OLD.invalid AND NOT OLD.deleted AND OLD.type = 1 -- FileInfoTypeDirectory
BEGIN
    UPDATE sizes SET directories = directories - 1, total_size = total_size - OLD.size
        WHERE folder_idx = OLD.folder_idx AND device_idx = OLD.device_idx;
END
;
CREATE TRIGGER IF NOT EXISTS sizes_delete_symlink AFTER DELETE ON files
WHEN NOT OLD.invalid AND NOT OLD.deleted AND OLD.type = 4 -- FileInfoTypeSymlink
BEGIN
    UPDATE sizes SET symlinks = symlinks - 1, total_size = total_size - OLD.size
        WHERE folder_idx = OLD.folder_idx AND device_idx = OLD.device_idx;
END
;

--- Global
CREATE TABLE IF NOT EXISTS globals (
    folder_idx INTEGER NOT NULL,
    device_idx INTEGER NOT NULL,
    file_sequence INTEGER NOT NULL,
    name TEXT NOT NULL,
    FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE,
    FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
    FOREIGN KEY(folder_idx, device_idx, file_sequence) REFERENCES files(folder_idx, device_idx, sequence) ON DELETE CASCADE
) STRICT
;
CREATE UNIQUE INDEX IF NOT EXISTS globals_seq ON globals (folder_idx, device_idx, file_sequence)
;
CREATE UNIQUE INDEX IF NOT EXISTS globals_name ON globals (folder_idx, name)
;

--- Needs
CREATE TABLE IF NOT EXISTS needs (
    folder_idx INTEGER NOT NULL,
    device_idx INTEGER NOT NULL,
    file_sequence INTEGER, -- deliberately nullable
    name TEXT NOT NULL,
    FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE,
    FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
    FOREIGN KEY(folder_idx, device_idx, file_sequence) REFERENCES files(folder_idx, device_idx, sequence) ON DELETE CASCADE
) STRICT
;
CREATE UNIQUE INDEX IF NOT EXISTS needs_file_sequence ON needs (folder_idx, device_idx, file_sequence)
;
CREATE UNIQUE INDEX IF NOT EXISTS needs_name ON needs (folder_idx, device_idx, name)
;

--- Blocks
CREATE TABLE IF NOT EXISTS blocks (
    hash 			TEXT NOT NULL,
    folder_idx 		INTEGER NOT NULL,
    device_idx 		INTEGER NOT NULL,
    file_sequence 	INTEGER NOT NULL,
    offset  		INTEGER NOT NULL,
    FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE,
    FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
    FOREIGN KEY(folder_idx, device_idx, file_sequence) REFERENCES files(folder_idx, device_idx, sequence) ON DELETE CASCADE
) STRICT
;
CREATE INDEX IF NOT EXISTS blocks_hash ON blocks (hash)
;
CREATE UNIQUE INDEX IF NOT EXISTS blocks_block ON blocks (folder_idx, device_idx, file_sequence, offset)
;

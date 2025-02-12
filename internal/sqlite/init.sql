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
    local_flags  INTEGER NOT NULL,
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
    type INTEGER NOT NULL,
    flag_bit INTEGER NOT NULL,
    count INTEGER NOT NULL,
    size INTEGER NOT NULL,
    PRIMARY KEY(folder_idx, device_idx, type, flag_bit),
    FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
    FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE
) STRICT
;

{{ range $type := $.FileInfoTypes }}
{{ range $flag := $.LocalFlagBits }}
CREATE TRIGGER IF NOT EXISTS sizes_insert_type{{$type}}_flag{{$flag}} AFTER INSERT ON files
WHEN NOT NEW.invalid AND NOT NEW.deleted AND NEW.type = {{$type}}
{{- if ne $flag 0 }}
AND NEW.local_flags & {{$flag}} != 0
{{- else }}
AND NEW.local_flags = 0
{{- end }}
BEGIN
    INSERT INTO sizes (folder_idx, device_idx, type, flag_bit, count, size)
        VALUES (NEW.folder_idx, NEW.device_idx, NEW.type, {{$flag}}, 1, NEW.size)
        ON CONFLICT DO UPDATE SET count = count + 1, size = size + NEW.size;
END
;
CREATE TRIGGER IF NOT EXISTS sizes_delete_type{{$type}}_flag{{$flag}} AFTER DELETE ON files
WHEN NOT OLD.invalid AND NOT OLD.deleted AND OLD.type = {{$type}}
{{- if ne $flag 0 }}
AND OLD.local_flags & {{$flag}} != 0
{{- else }}
AND OLD.local_flags = 0
{{- end }}
BEGIN
    UPDATE sizes SET count = count - 1, size = size - OLD.size
        WHERE folder_idx = OLD.folder_idx AND device_idx = OLD.device_idx AND type = {{$type}} AND flag_bit = {{$flag}};
END
;
{{ end }}
{{ end }}

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

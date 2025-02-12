-- Sizes
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

--- Maintain size counts when files are added and removed using triggers

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
CREATE TRIGGER IF NOT EXISTS sizes_update_type{{$type}}_flag{{$flag}}_add AFTER UPDATE ON files
WHEN NOT NEW.invalid AND NOT NEW.deleted AND NEW.type = {{$type}} AND NEW.local_flags != OLD.local_flags
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
CREATE TRIGGER IF NOT EXISTS sizes_update_type{{$type}}_flag{{$flag}}_del AFTER UPDATE ON files
WHEN NOT OLD.invalid AND NOT OLD.deleted AND OLD.type = {{$type}} AND NEW.local_flags != OLD.local_flags
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

-- add the sequence column to indexids
ALTER TABLE indexids ADD COLUMN sequence INTEGER NOT NULL DEFAULT 0
;
UPDATE indexids SET sequence = (
    SELECT COALESCE(MAX(remote_sequence), MAX(sequence)) FROM files
        WHERE files.device_idx = indexids.device_idx AND files.folder_idx = indexids.folder_idx
)
;

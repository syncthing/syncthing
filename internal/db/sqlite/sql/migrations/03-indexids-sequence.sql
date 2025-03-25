-- add the sequence column to indexids
ALTER TABLE indexids ADD COLUMN sequence INTEGER NOT NULL DEFAULT 0
;
UPDATE indexids SET sequence = ff.sequence
    FROM (SELECT device_idx, folder_idx, COALESCE(MAX(remote_sequence), MAX(sequence)) AS sequence FROM files GROUP BY device_idx, folder_idx) AS ff
    WHERE indexids.device_idx = ff.device_idx AND indexids.folder_idx = ff.folder_idx
;

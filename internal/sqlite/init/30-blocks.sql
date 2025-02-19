--- Blocks
CREATE TABLE IF NOT EXISTS blocks (
    hash 			TEXT NOT NULL,
    folder_idx 		INTEGER NOT NULL,
    device_idx 		INTEGER NOT NULL,
    file_sequence 	INTEGER NOT NULL,
    idx             INTEGER NOT NULL,
    offset          INTEGER NOT NULL,
    size            INTEGER NOT NULL,
    FOREIGN KEY(folder_idx) REFERENCES folders(idx) ON DELETE CASCADE,
    FOREIGN KEY(device_idx) REFERENCES devices(idx) ON DELETE CASCADE,
    FOREIGN KEY(file_sequence) REFERENCES files(sequence) ON DELETE CASCADE
) STRICT
;
CREATE INDEX IF NOT EXISTS blocks_hash ON blocks (hash)
;
CREATE UNIQUE INDEX IF NOT EXISTS blocks_block ON blocks (folder_idx, device_idx, file_sequence, idx)
;

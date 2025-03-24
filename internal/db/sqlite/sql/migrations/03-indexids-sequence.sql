-- add the sequence column to indexids
ALTER TABLE indexids ADD COLUMN sequence INTEGER NOT NULL DEFAULT 0
;

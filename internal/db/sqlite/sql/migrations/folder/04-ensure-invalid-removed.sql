-- Copyright (C) 2025 The Syncthing Authors.
--
-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this file,
-- You can obtain one at https://mozilla.org/MPL/2.0/.

-- Ensure the invalid column is properly handled in the files table
-- This is a safety check to prevent "NOT NULL constraint failed: files.invalid" errors
-- This migration handles databases that still have the invalid column from older versions

-- Update any existing entries to move the invalid flag to local_flags
-- This ensures backward compatibility with older database schemas
-- We use a simple approach that works with most SQLite versions
-- This preserves the invalid status by moving it to the local_flags column
UPDATE files 
SET local_flags = local_flags | {{.FlagLocalRemoteInvalid}}
WHERE invalid = 1;

-- Note: We cannot safely drop the column here because SQLite's support for
-- ALTER TABLE DROP COLUMN varies by version. The application code will
-- handle the column appropriately based on its existence.
-- This approach ensures compatibility across different SQLite versions

-- Force a new index transmission to ensure consistency
-- This ensures that all devices will re-sync their indexes to reflect the migration
DELETE FROM indexids;
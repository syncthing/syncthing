-- Copyright (C) 2025 The Syncthing Authors.
--
-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this file,
-- You can obtain one at https://mozilla.org/MPL/2.0/.

-- Remove broken file entries in the database.
DELETE FROM files
    WHERE type == 0 -- files
        AND NOT deleted -- that are not deleted
        AND blocklist_hash IS null -- with no blocks
        AND local_flags & {{.LocalInvalidFlags}} == 0 -- and not invalid
;

-- Force a new index transmission.
DELETE FROM indexids
;

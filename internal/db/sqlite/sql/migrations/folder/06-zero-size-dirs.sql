-- Copyright (C) 2026 The Syncthing Authors.
--
-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this file,
-- You can obtain one at https://mozilla.org/MPL/2.0/.

-- All non-file entries should have size zero.
-- Directories were previously stored with a "synthetic" size of 128.
UPDATE files
    SET size = 0
    WHERE type != 0
;
UPDATE counts
    SET size = 0
    WHERE type != 0
;

-- Copyright (C) 2026 The Syncthing Authors.
--
-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this file,
-- You can obtain one at https://mozilla.org/MPL/2.0/.

-- The schema creates files_device_remote_sequence without the old non-NULL
-- predicate. Its implicit rowid supports local sequence iteration, replacing
-- the separate device/sequence index.
DROP INDEX IF EXISTS files_remote_sequence
;
DROP INDEX IF EXISTS files_device_sequence
;

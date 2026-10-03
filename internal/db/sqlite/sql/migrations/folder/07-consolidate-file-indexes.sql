-- Copyright (C) 2026 The Syncthing Authors.
--
-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this file,
-- You can obtain one at https://mozilla.org/MPL/2.0/.

-- The schema creates files_name_device and files_device_sequence. These
-- replace the old device/name index and the separate name index.
DROP INDEX IF EXISTS files_device_name
;
DROP INDEX IF EXISTS files_name_idx_only
;

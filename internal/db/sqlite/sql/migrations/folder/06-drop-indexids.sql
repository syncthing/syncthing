-- Copyright (C) 2025 The Syncthing Authors.
--
-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this file,
-- You can obtain one at https://mozilla.org/MPL/2.0/.

-- Drop all index IDs, because we cannot tell which ones are affected by
-- https://github.com/syncthing/syncthing/issues/10469 and which are not.
DELETE FROM indexids

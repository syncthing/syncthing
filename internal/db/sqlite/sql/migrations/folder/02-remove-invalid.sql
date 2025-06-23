-- Copyright (C) 2025 The Syncthing Authors.
--
-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this file,
-- You can obtain one at https://mozilla.org/MPL/2.0/.

-- Remote files with the invalid bit instead gain the RemoteInvalid local
-- flag.
UPDATE files
    SET local_flags = local_flags | {{.FlagLocalRemoteInvalid}}
    FROM (
        SELECT idx FROM devices
        WHERE device_id = '7777777-777777N-7777777-777777N-7777777-777777N-7777777-77777Q4'
    ) AS local_device
    WHERE invalid AND device_idx != local_device.idx
;

-- The invalid column goes away.
ALTER TABLE files DROP COLUMN invalid
;

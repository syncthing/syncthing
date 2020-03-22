// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"github.com/syncthing/syncthing/lib/protocol"
)

func ListPendingDevices(db *Lowlevel) (map[protocol.DeviceID]ObservedDevice, error) {
	iter, err := db.NewPrefixIterator([]byte{KeyTypePendingDevice})
	if err != nil {
		return nil, err
	}
	res := make(map[protocol.DeviceID]ObservedDevice)
	defer iter.Release()
	for iter.Next() {
		bs, err := db.Get(iter.Key())
		if err != nil {
			return nil, err
		}
		deviceID, ok := db.keyer.DeviceFromPendingDeviceKey(iter.Key())
		if !ok {
			// Not having the device in the index is bad. Clear it. FIXME?
			if err := db.Delete(iter.Key()); err != nil {
				return nil, err
			}
			continue
		}
		var od ObservedDevice
		od.Unmarshal(bs)//FIXME error check?
		res[protocol.DeviceIDFromBytes(deviceID)] = od
	}
	return res, nil
}

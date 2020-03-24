// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

func (db *Lowlevel) AddOrUpdatePendingDevice(device protocol.DeviceID, name, address string) {
	//FIXME locking? m.mut.Lock()
	//FIXME locking? defer m.mut.Unlock()

	key := db.keyer.GeneratePendingDeviceKey(nil, device[:])
	timestamp := time.Now().Round(time.Second)
	od := ObservedDevice{
		Time: &timestamp,
		Name: name,
		Address: address,
	}
	bs, err := od.Marshal()
	if err != nil {
		//FIXME
		return
	}
	err = db.Put(key, bs)
	if err != nil {
		//FIXME
		return
	}
}

func (db *Lowlevel) PendingDevices() (map[protocol.DeviceID]ObservedDevice, error) {
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
		deviceID := db.keyer.DeviceFromPendingDeviceKey(iter.Key())
		if len(deviceID) != protocol.DeviceIDLength {
			l.Warnln("Invalid pending device ID, deleting from database")
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

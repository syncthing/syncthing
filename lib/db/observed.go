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
	od := ObservedDevice{
		Time:    time.Now().Round(time.Second),
		Name:    name,
		Address: address,
	}
	bs, err := od.Marshal()
	if err == nil {
		err = db.Put(key, bs)
	}
	if err != nil {
		l.Warnf("Failed to store pending device entry: %v", err)
	}
}

// List unknown devices that tried to connect.  As a side-effect, any
// invalid entries are dropped from the database after a warning log
// message.  That's the only possible "repair" measure and appropriate
// for the importance of pending entries.  They will come back soon if
// still relevant.
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
		var od ObservedDevice
		deviceID := db.keyer.DeviceFromPendingDeviceKey(iter.Key())
		//FIXME: DeviceIDFromBytes() panics when given a wrong length input.
		//       It should rather return an error which we'd check for here.
		if len(deviceID) != protocol.DeviceIDLength {
			goto deleteKey
		}
		if err := od.Unmarshal(bs); err != nil {
			goto deleteKey
		}
		res[protocol.DeviceIDFromBytes(deviceID)] = od
		continue
	deleteKey:
		l.Warnln("Invalid pending device entry, deleting from database")
		if err := db.Delete(iter.Key()); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (db *Lowlevel) RemovePendingDevice(device protocol.DeviceID) {
	key := db.keyer.GeneratePendingDeviceKey(nil, device[:])
	if err := db.Delete(key); err != nil {
		l.Warnf("Failed to remove pending device entry: %v", err)
	}
}

func (db *Lowlevel) AddOrUpdatePendingFolder(id, label string, device protocol.DeviceID) {
	//FIXME locking? m.mut.Lock()
	//FIXME locking? defer m.mut.Unlock()

	key, err := db.keyer.GeneratePendingFolderKey(nil, []byte(id), device[:])
	if err == nil {
		of := ObservedFolder{
			Time:  time.Now().Round(time.Second),
			Label: label,
		}
		bs, err := of.Marshal()
		if err == nil {
			err = db.Put(key, bs)
		}
	}
	if err != nil {
		l.Warnf("Failed to store pending folder entry: %v", err)
	}
}

// List folders that we don't yet share with the offering devices.  As
// a side-effect, any invalid entries are dropped from the database
// after a warning log message.  That's the only possible "repair"
// measure and appropriate for the importance of pending entries.
// They will come back soon if still relevant.
func (db *Lowlevel) PendingFolders(device protocol.DeviceID) (map[string]map[protocol.DeviceID]ObservedFolder, error) {
	iter, err := db.NewPrefixIterator([]byte{KeyTypePendingFolder})
	if err != nil {
		return nil, err
	}
	res := make(map[string]map[protocol.DeviceID]ObservedFolder)
	defer iter.Release()
	for iter.Next() {
		bs, err := db.Get(iter.Key())
		if err != nil {
			return nil, err
		}
		if keyDev, ok := db.keyer.DeviceFromPendingFolderKey(iter.Key()); ok {
			// Here we expect the length to match, coming from the device index.
			deviceID := protocol.DeviceIDFromBytes(keyDev)
			if device != protocol.EmptyDeviceID && device != deviceID {
				continue
			}
			var of ObservedFolder
			folderID := string(db.keyer.FolderFromPendingFolderKey(iter.Key()))
			if len(folderID) < 1 {
				goto deleteKey
			}
			if err := of.Unmarshal(bs); err != nil {
				goto deleteKey
			}
			if _, ok := res[folderID]; !ok {
				res[folderID] = make(map[protocol.DeviceID]ObservedFolder)
			}
			res[folderID][deviceID] = of
			continue
		}
	deleteKey:
		l.Warnln("Invalid pending folder entry, deleting from database")
		if err := db.Delete(iter.Key()); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (db *Lowlevel) RemovePendingFolder(id string, devices []protocol.DeviceID) {
	for _, dev := range devices {
		key, err := db.keyer.GeneratePendingFolderKey(nil, []byte(id), dev[:])
		if err == nil {
			if err := db.Delete(key); err != nil {
				l.Warnf("Failed to remove pending folder entry: %v", err)
			}
		}
	}
}

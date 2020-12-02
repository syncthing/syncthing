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

func (db *Lowlevel) AddOrUpdatePendingDevice(device protocol.DeviceID, name, address string) error {
	key := db.keyer.GeneratePendingDeviceKey(nil, device[:])
	od := ObservedDevice{
		Time:    time.Now().Round(time.Second),
		Name:    name,
		Address: address,
	}
	bs, err := od.Marshal()
	if err != nil {
		return err
	}
	return db.Put(key, bs)
}

func (db *Lowlevel) RemovePendingDevice(device protocol.DeviceID) {
	key := db.keyer.GeneratePendingDeviceKey(nil, device[:])
	if err := db.Delete(key); err != nil {
		l.Warnf("Failed to remove pending device entry: %v", err)
	}
}

// PendingDevices enumerates all entries.  Invalid ones are dropped from the database
// after a warning log message, as a side-effect.
func (db *Lowlevel) PendingDevices() (map[protocol.DeviceID]ObservedDevice, error) {
	iter, err := db.NewPrefixIterator([]byte{KeyTypePendingDevice})
	if err != nil {
		return nil, err
	}
	defer iter.Release()
	res := make(map[protocol.DeviceID]ObservedDevice)
	for iter.Next() {
		keyDev := db.keyer.DeviceFromPendingDeviceKey(iter.Key())
		deviceID, err := protocol.DeviceIDFromBytes(keyDev)
		var bs []byte
		var od ObservedDevice
		if err != nil {
			goto deleteKey
		}
		if bs, err = db.Get(iter.Key()); err != nil {
			goto deleteKey
		}
		if err = od.Unmarshal(bs); err != nil {
			goto deleteKey
		}
		res[deviceID] = od
		continue
	deleteKey:
		// Deleting invalid entries is the only possible "repair" measure and
		// appropriate for the importance of pending entries.  They will come back
		// soon if still relevant.
		l.Infof("Invalid pending device entry, deleting from database: %x", iter.Key())
		if err := db.Delete(iter.Key()); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (db *Lowlevel) AddOrUpdatePendingFolder(id, label string, device protocol.DeviceID) error {
	key, err := db.keyer.GeneratePendingFolderKey(nil, device[:], []byte(id))
	if err != nil {
		return err
	}
	of := ObservedFolder{
		Time:  time.Now().Round(time.Second),
		Label: label,
	}
	bs, err := of.Marshal()
	if err != nil {
		return err
	}
	return db.Put(key, bs)
}

// RemovePendingFolderForDevice removes entries for specific folder / device combinations.
func (db *Lowlevel) RemovePendingFolderForDevice(id string, device protocol.DeviceID) {
	key, err := db.keyer.GeneratePendingFolderKey(nil, device[:], []byte(id))
	if err != nil {
		return
	}
	if err := db.Delete(key); err != nil {
		l.Warnf("Failed to remove pending folder entry: %v", err)
	}
}

// RemovePendingFolder removes all entries matching a specific folder ID.
func (db *Lowlevel) RemovePendingFolder(id string) {
	iter, err := db.NewPrefixIterator([]byte{KeyTypePendingFolder})
	if err != nil {
		l.Warnf("Could not iterate through pending folder entries: %v", err)
		return
	}
	defer iter.Release()
	for iter.Next() {
		if id != string(db.keyer.FolderFromPendingFolderKey(iter.Key())) {
			continue
		}
		if err := db.Delete(iter.Key()); err != nil {
			l.Warnf("Failed to remove pending folder entry: %v", err)
		}
	}
}

// Consolidated information about a pending folder
type PendingFolder struct {
	OfferedBy map[protocol.DeviceID]ObservedFolder `json:"offeredBy"`
}

func (db *Lowlevel) PendingFolders() (map[string]PendingFolder, error) {
	return db.PendingFoldersForDevice(protocol.EmptyDeviceID)
}

// PendingFoldersForDevice enumerates only entries matching the given device ID, unless it
// is EmptyDeviceID.  Invalid ones are dropped from the database after a warning log
// message, as a side-effect.
func (db *Lowlevel) PendingFoldersForDevice(device protocol.DeviceID) (map[string]PendingFolder, error) {
	var err error
	prefixKey := []byte{KeyTypePendingFolder}
	if device != protocol.EmptyDeviceID {
		prefixKey, err = db.keyer.GeneratePendingFolderKey(nil, device[:], nil)
		if err != nil {
			return nil, err
		}
	}
	iter, err := db.NewPrefixIterator(prefixKey)
	if err != nil {
		return nil, err
	}
	defer iter.Release()
	res := make(map[string]PendingFolder)
	for iter.Next() {
		keyDev, ok := db.keyer.DeviceFromPendingFolderKey(iter.Key())
		deviceID, err := protocol.DeviceIDFromBytes(keyDev)
		var of ObservedFolder
		var folderID string
		var bs []byte
		if !ok || err != nil {
			goto deleteKey
		}
		if folderID = string(db.keyer.FolderFromPendingFolderKey(iter.Key())); len(folderID) < 1 {
			goto deleteKey
		}
		if bs, err = db.Get(iter.Key()); err != nil {
			goto deleteKey
		}
		if err = of.Unmarshal(bs); err != nil {
			goto deleteKey
		}
		if _, ok := res[folderID]; !ok {
			res[folderID] = PendingFolder{
				OfferedBy: map[protocol.DeviceID]ObservedFolder{},
			}
		}
		res[folderID].OfferedBy[deviceID] = of
		continue
	deleteKey:
		// Deleting invalid entries is the only possible "repair" measure and
		// appropriate for the importance of pending entries.  They will come back
		// soon if still relevant.
		l.Infof("Invalid pending folder entry, deleting from database: %x", iter.Key())
		if err := db.Delete(iter.Key()); err != nil {
			return nil, err
		}
	}
	return res, nil
}

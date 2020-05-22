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
	if err == nil {
		err = db.Put(key, bs)
	}
	if err != nil {
		l.Warnf("Failed to store pending device entry: %v", err)
	}
	return err
}

// PendingDevices drops any invalid entries from the database after a
// warning log message, as a side-effect.  That's the only possible
// "repair" measure and appropriate for the importance of pending
// entries.  They will come back soon if still relevant.
func (db *Lowlevel) PendingDevices() (map[protocol.DeviceID]ObservedDevice, error) {
	iter, err := db.NewPrefixIterator([]byte{KeyTypePendingDevice})
	if err != nil {
		return nil, err
	}
	defer iter.Release()
	res := make(map[protocol.DeviceID]ObservedDevice)
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
		l.Warnf("Invalid pending device entry, deleting from database: %x", iter.Key())
		if err := db.Delete(iter.Key()); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (db *Lowlevel) AddOrUpdatePendingFolder(id, label string, device protocol.DeviceID) error {
	key, err := db.keyer.GeneratePendingFolderKey(nil, device[:], []byte(id))
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
	return err
}

// PendingFolders drops any invalid entries from the database as a side-effect.
func (db *Lowlevel) PendingFolders() (map[string]map[protocol.DeviceID]ObservedFolder, error) {
	iter, err := db.NewPrefixIterator([]byte{KeyTypePendingFolder})
	if err != nil {
		return nil, err
	}
	defer iter.Release()
	res := make(map[string]map[protocol.DeviceID]ObservedFolder)
	for iter.Next() {
		keyDev, ok := db.keyer.DeviceFromPendingFolderKey(iter.Key())
		if !ok {
			if err := db.deleteInvalidPendingFolder(iter.Key()); err != nil {
				return nil, err
			}
			continue
		}
		// Here we expect the length to match, coming from the device index.
		deviceID := protocol.DeviceIDFromBytes(keyDev)
		if err := db.collectPendingFolder(iter.Key(), deviceID, res); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// PendingFoldersForDevice drops any invalid entries from the database as a side-effect.
func (db *Lowlevel) PendingFoldersForDevice(device protocol.DeviceID) (map[string]map[protocol.DeviceID]ObservedFolder, error) {
	prefixKey, err := db.keyer.GeneratePendingFolderKey(nil, device[:], nil)
	if err != nil {
		return nil, err
	}
	iter, err := db.NewPrefixIterator(prefixKey)
	if err != nil {
		return nil, err
	}
	defer iter.Release()
	res := make(map[string]map[protocol.DeviceID]ObservedFolder)
	for iter.Next() {
		if err := db.collectPendingFolder(iter.Key(), device, res); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (db *Lowlevel) collectPendingFolder(key []byte, device protocol.DeviceID, res map[string]map[protocol.DeviceID]ObservedFolder) error {
	bs, err := db.Get(key)
	if err != nil {
		return err
	}
	var of ObservedFolder
	folderID := string(db.keyer.FolderFromPendingFolderKey(key))
	if len(folderID) < 1 {
		return db.deleteInvalidPendingFolder(key)
	}
	if err := of.Unmarshal(bs); err != nil {
		return db.deleteInvalidPendingFolder(key)
	}
	if _, ok := res[folderID]; !ok {
		res[folderID] = make(map[protocol.DeviceID]ObservedFolder)
	}
	res[folderID][device] = of
	return nil
}

// deleteInvalidPendingFolder logs a warning message before dropping the given entry.
// That's the only possible "repair" measure and appropriate for the importance of pending
// entries.  They will come back soon if still relevant.
func (db *Lowlevel) deleteInvalidPendingFolder(key []byte) error {
	l.Warnf("Invalid pending folder entry, deleting from database: %x", key)
	err := db.Delete(key)
	return err
}

// Set of devices for which pending folders are allowed, but not specific folder IDs
type DropListObserved map[protocol.DeviceID]map[string]struct{}

func NewDropListObserved() DropListObserved {
	return make(DropListObserved)
}

func (dl DropListObserved) MarkDevice(device protocol.DeviceID) map[string]struct{} {
	folders, ok := dl[device]
	if !ok {
		dl[device] = nil
	}
	return folders
}

func (dl DropListObserved) UnmarkDevice(device protocol.DeviceID) {
	delete(dl, device)
}

func (dl DropListObserved) MarkFolder(folder string, devices []protocol.DeviceID) {
	for _, dev := range devices {
		folders := dl.MarkDevice(dev)
		if folders == nil {
			folders = make(map[string]struct{})
			dl[dev] = folders
		}
		folders[folder] = struct{}{}
	}
}

// shouldDropPendingDevice defines how the drop-list is interpreted for pending devices
func (dl DropListObserved) shouldDropPendingDevice(key []byte, keyer keyer) bool {
	keyDev := keyer.DeviceFromPendingDeviceKey(key)
	//FIXME: DeviceIDFromBytes() panics when given a wrong length input.
	//       It should rather return an error which we'd check for here.
	if len(keyDev) != protocol.DeviceIDLength {
		l.Warnf("Invalid pending device entry, deleting from database: %x", key)
		return true
	}
	// Valid entries are looked up in the drop-list, invalid ones cleaned up
	deviceID := protocol.DeviceIDFromBytes(keyDev)
	_, dropDevice := dl[deviceID]
	if dropDevice {
		l.Debugf("Removing marked pending device %v", deviceID)
	}
	return dropDevice
}

// shouldDropPendingFolder defines how the drop-list is interpreted for pending folders,
// which is different and more nested than for devices
func (dl DropListObserved) shouldDropPendingFolder(key []byte, keyer keyer) bool {
	keyDev, ok := keyer.DeviceFromPendingFolderKey(key)
	if !ok {
		l.Warnf("Invalid pending folder entry, deleting from database: %x", key)
		return true
	}
	// Valid entries are looked up in the drop-list, invalid ones cleaned up
	deviceID := protocol.DeviceIDFromBytes(keyDev)
	dropFolders, allowDevice := dl[deviceID]
	// Check the associated set of folders if provided, otherwise drop.
	if !allowDevice {
		l.Debugf("Removing pending folder offered by %v", deviceID)
		return true
	}
	if len(dropFolders) == 0 {
		// Map is empty or nil, skip lookup
		return false
	}
	folderID := keyer.FolderFromPendingFolderKey(key)
	// Drop only mentioned folder IDs
	_, dropFolder := dropFolders[string(folderID)]
	if dropFolder {
		l.Debugf("Removing marked pending folder %s for %v", folderID, deviceID)
	}
	return dropFolder
}

// CleanPendingDevices removes all pending device entries matching a given set of device IDs
func (db *Lowlevel) CleanPendingDevices(dropList DropListObserved) {
	iter, err := db.NewPrefixIterator([]byte{KeyTypePendingDevice})
	if err != nil {
		l.Warnf("Could not iterate through pending device entries for cleanup: %v", err)
		return
	}
	defer iter.Release()
	for iter.Next() {
		if dropList.shouldDropPendingDevice(iter.Key(), db.keyer) {
			if err := db.Delete(iter.Key()); err != nil {
				l.Warnf("Failed to remove pending device entry: %v", err)
			}
		}
	}
}

// CleanPendingFolders removes all pending folder entries not matching a given set of
// device IDs, or matching the set of folder IDs associated with those given devices.
func (db *Lowlevel) CleanPendingFolders(dropList DropListObserved) {
	iter, err := db.NewPrefixIterator([]byte{KeyTypePendingFolder})
	if err != nil {
		l.Warnf("Could not iterate through pending folder entries for cleanup: %v", err)
		return
	}
	defer iter.Release()
	for iter.Next() {
		if dropList.shouldDropPendingFolder(iter.Key(), db.keyer) {
			if err := db.Delete(iter.Key()); err != nil {
				l.Warnf("Failed to remove pending folder entry: %v", err)
			}
		}
	}
}

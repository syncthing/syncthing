// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"time"

	"github.com/syncthing/syncthing/lib/db/backend"
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
	return err
}

func (db *Lowlevel) RemovePendingDevice(device protocol.DeviceID) {
	key := db.keyer.GeneratePendingDeviceKey(nil, device[:])
	if err := db.Delete(key); err != nil {
		l.Warnf("Failed to remove pending device entry: %v", err)
	}
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
	if err == nil {
		err = db.Put(key, bs)
	}
	return err
}

// RemovePendingFolder removes entries for specific folder / device combinations
func (db *Lowlevel) RemovePendingFolder(id string, device protocol.DeviceID) {
	key, err := db.keyer.GeneratePendingFolderKey(nil, []byte(id), device[:])
	if err == nil {
		if err = db.Delete(key); err == nil {
			return
		}
	}
	l.Warnf("Failed to remove pending folder entry: %v", err)
}

// PendingDeviceIterator abstracts away the key handling and validation, yielding only
// valid entries.  Release() must be called on it when no longer needed.
type PendingDeviceIterator interface {
	Release()
	NextValid() bool
	Forget()
	DeviceID() protocol.DeviceID
	Observed() ObservedDevice
}

// PendingFolderIterator abstracts away the key handling and validation, yielding only
// valid entries.  Release() must be called on it when no longer needed.
type PendingFolderIterator interface {
	Release()
	NextValid() bool
	Forget()
	DeviceID() protocol.DeviceID
	FolderID() string
	Observed() ObservedFolder
}

// pendingDeviceIterator caches the current entry's data after validation
type pendingDeviceIterator struct {
	backend.Iterator

	db       *Lowlevel
	deviceID protocol.DeviceID
	device   ObservedDevice
}

// pendingFolderIterator caches the current entry's data after validation.  Embeds a
// pendingDeviceIterator to reuse some common method implementations.
type pendingFolderIterator struct {
	pendingDeviceIterator

	folderID string
	folder   ObservedFolder
}

// NewPendingDeviceIterator allows to iterate over all pending device entries, including
// automatic removal of invalid entries.
func (db *Lowlevel) NewPendingDeviceIterator() (PendingDeviceIterator, error) {
	iter, err := db.NewPrefixIterator([]byte{KeyTypePendingDevice})
	if err != nil {
		l.Infof("Could not iterate through pending device entries: %v", err)
		return nil, err
	}
	var res pendingDeviceIterator
	res.Iterator = iter
	res.db = db
	return &res, nil
}

// NewPendingDeviceIterator allows to iterate over all pending folder entries, including
// automatic removal of invalid entries.  Optionally limit to entries matching a given
// device ID.
func (db *Lowlevel) NewPendingFolderIterator(device []byte) (PendingFolderIterator, error) {
	prefixKey := []byte{KeyTypePendingFolder}
	if len(device) > 0 {
		if _, err := db.keyer.GeneratePendingFolderKey(prefixKey, device, nil); err != nil {
			return nil, err
		}
	}
	iter, err := db.NewPrefixIterator(prefixKey)
	if err != nil {
		l.Infof("Could not iterate through pending folder entries: %v", err)
		return nil, err
	}
	var res pendingFolderIterator
	res.Iterator = iter
	res.db = db
	return &res, nil
}

// NextValid discards invalid entries after logging a message.  That's the only possible
// "repair" measure and appropriate for the importance of pending entries.  They will come
// back soon if still relevant.
func (iter *pendingDeviceIterator) NextValid() bool {
	for iter.Next() {
		keyDev := iter.db.keyer.DeviceFromPendingDeviceKey(iter.Key())
		deviceID, err := protocol.DeviceIDFromBytes(keyDev)
		var bs []byte
		if err != nil {
			goto deleteKey
		}
		if bs, err = iter.db.Get(iter.Key()); err != nil {
			goto deleteKey
		}
		if err := iter.device.Unmarshal(bs); err != nil {
			goto deleteKey
		}
		iter.deviceID = deviceID
		return true
	deleteKey:
		l.Infof("Invalid pending device entry, deleting from database: %x", iter.Key())
		iter.Forget()
	}
	return false
}

// NextValid discards invalid entries after logging a message.  That's the only possible
// "repair" measure and appropriate for the importance of pending entries.  They will come
// back soon if still relevant.
func (iter *pendingFolderIterator) NextValid() bool {
	for iter.Next() {
		keyDev, ok := iter.db.keyer.DeviceFromPendingFolderKey(iter.Key())
		deviceID, err := protocol.DeviceIDFromBytes(keyDev)
		var folderID string
		var bs []byte
		if !ok || err != nil {
			goto deleteKey
		}
		if folderID = string(iter.db.keyer.FolderFromPendingFolderKey(iter.Key())); len(folderID) < 1 {
			goto deleteKey
		}
		if bs, err = iter.db.Get(iter.Key()); err != nil {
			goto deleteKey
		}
		if err := iter.folder.Unmarshal(bs); err != nil {
			goto deleteKey
		}
		iter.deviceID = deviceID
		iter.folderID = folderID
		return true
	deleteKey:
		l.Infof("Invalid pending folder entry, deleting from database: %x", iter.Key())
		iter.Forget()
	}
	return false
}

func (iter pendingDeviceIterator) DeviceID() protocol.DeviceID {
	return iter.deviceID
}

func (iter pendingFolderIterator) FolderID() string {
	return iter.folderID
}

func (iter pendingDeviceIterator) Observed() ObservedDevice {
	return iter.device
}

func (iter pendingFolderIterator) Observed() ObservedFolder {
	return iter.folder
}

func (iter pendingDeviceIterator) Forget() {
	if err := iter.db.Delete(iter.Key()); err != nil {
		l.Warnf("Failed to remove pending entry: %v", err)
		return
	}
}

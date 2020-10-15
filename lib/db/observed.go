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
	return err
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

func (db *Lowlevel) NewPendingDeviceIterator() (PendingDeviceIterator, error) {
	var res pendingDeviceIterator
	iter, err := db.NewPrefixIterator([]byte{KeyTypePendingDevice})
	if err != nil {
		l.Infof("Could not iterate through pending device entries: %v", err)
		return res, err
	}
	res.Iterator = iter
	res.db = db
	return res, nil
}

func (db *Lowlevel) NewPendingFolderIterator() (PendingFolderIterator, error) {
	prefixKey := []byte{KeyTypePendingFolder}
	return db.createPendingFolderIterator(prefixKey)
}

func (db *Lowlevel) NewPendingFolderForDeviceIterator(device protocol.DeviceID) (PendingFolderIterator, error) {
	prefixKey, err := db.keyer.GeneratePendingFolderKey(nil, device[:], nil)
	if err != nil {
		return nil, err
	}
	return db.createPendingFolderIterator(prefixKey)
}

func (db *Lowlevel) createPendingFolderIterator(prefixKey []byte) (PendingFolderIterator, error) {
	var res pendingFolderIterator
	iter, err := db.NewPrefixIterator(prefixKey)
	if err != nil {
		l.Infof("Could not iterate through pending folder entries: %v", err)
		return res, err
	}
	res.Iterator = iter
	res.db = db
	return res, nil
}

// NextValid discards invalid entries after logging a message.  That's the only possible
// "repair" measure and appropriate for the importance of pending entries.  They will come
// back soon if still relevant.
func (iter pendingDeviceIterator) NextValid() bool {
	for iter.Iterator.Next() {
		keyDev := iter.db.keyer.DeviceFromPendingDeviceKey(iter.Key())
		deviceID, err := protocol.DeviceIDFromBytes(keyDev)
		var bs []byte
		if err != nil {
			goto deleteKey
		}
		bs, err = iter.db.Get(iter.Key())
		if err != nil {
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
func (iter pendingFolderIterator) NextValid() bool {
	for iter.Iterator.Next() {
		keyDev, ok := iter.db.keyer.DeviceFromPendingFolderKey(iter.Key())
		deviceID, err := protocol.DeviceIDFromBytes(keyDev)
		var bs []byte
		if !ok || err != nil {
			goto deleteKey
		}
		bs, err = iter.db.Get(iter.Key())
		if err != nil {
			goto deleteKey
		}
		if len(iter.FolderID()) < 1 {
			goto deleteKey
		}
		if err := iter.folder.Unmarshal(bs); err != nil {
			goto deleteKey
		}
		iter.deviceID = deviceID
		iter.folderID = string(iter.db.keyer.FolderFromPendingFolderKey(iter.Key()))
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
	l.Debugf("Removing pending device / folder %v", iter.deviceID)
	if err := iter.db.Delete(iter.Key()); err != nil {
		l.Warnf("Failed to remove pending entry: %v", err)
		return
	}
}

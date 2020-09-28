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
		keyDev := db.keyer.DeviceFromPendingDeviceKey(iter.Key())
		deviceID, err := protocol.DeviceIDFromBytes(keyDev)
		if err != nil {
			goto deleteKey
		}
		if err := od.Unmarshal(bs); err != nil {
			goto deleteKey
		}
		res[deviceID] = od
		continue
	deleteKey:
		l.Infof("Invalid pending device entry, deleting from database: %x", iter.Key())
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
		deviceID, err := protocol.DeviceIDFromBytes(keyDev)
		if !ok || err != nil {
			if err := db.deleteInvalidPendingFolder(iter.Key()); err != nil {
				return nil, err
			}
			continue
		}
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
	l.Infof("Invalid pending folder entry, deleting from database: %x", key)
	err := db.Delete(key)
	return err
}


type PendingDeviceIterator interface {
	backend.Iterator
	NextValid() bool
	DeviceID() protocol.DeviceID
	Forget()
}

type PendingFolderIterator interface {
	PendingDeviceIterator
	FolderID() string
}

type pendingDeviceIterator struct {
	backend.Iterator

	db       *Lowlevel
	deviceID protocol.DeviceID
}

type pendingFolderIterator struct {
	pendingDeviceIterator

	folderID string
}

func (db *Lowlevel) NewPendingDeviceIterator() PendingDeviceIterator {
	var res pendingDeviceIterator

	iter, err := db.NewPrefixIterator([]byte{KeyTypePendingDevice})
	if err != nil {
		l.Infof("Could not iterate through pending device entries for cleanup: %v", err)
		return res
	}
	res.Iterator = iter
	res.db = db
	return res
}

func (db *Lowlevel) NewPendingFolderIterator() PendingFolderIterator {
	var res pendingFolderIterator
	iter, err := db.NewPrefixIterator([]byte{KeyTypePendingFolder})
	if err != nil {
		l.Infof("Could not iterate through pending folder entries for cleanup: %v", err)
		return res
	}
	res.Iterator = iter
	res.db = db
	return res
}

// NextValid discards invalid entries after logging a message.  That's the only possible
// "repair" measure and appropriate for the importance of pending entries.  They will come
// back soon if still relevant.
func (iter pendingDeviceIterator) NextValid() bool {
	for iter.Iterator.Next() {
		keyDev := iter.db.keyer.DeviceFromPendingDeviceKey(iter.Key())
		deviceID, err := protocol.DeviceIDFromBytes(keyDev)
		if err != nil {
			l.Infof("Invalid pending device entry, deleting from database: %x", iter.Key())
			iter.Forget()
			continue
		}
		iter.deviceID = deviceID
		return true
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
		if !ok || err != nil {
			l.Infof("Invalid pending folder entry, deleting from database: %x", iter.Key())
			iter.Forget()
			continue
		}
		iter.deviceID = deviceID
		iter.folderID = string(iter.db.keyer.FolderFromPendingFolderKey(iter.Key()))
		return true
	}
	return false
}

func (iter pendingDeviceIterator) DeviceID() protocol.DeviceID {
	return iter.deviceID
}

func (iter pendingFolderIterator) FolderID() string {
	return iter.folderID
}

func (iter pendingDeviceIterator) Forget() {
	l.Debugf("Removing pending device / folder %v", iter.deviceID)
	if err := iter.db.Delete(iter.Key()); err != nil {
		l.Warnf("Failed to remove pending entry: %v", err)
		return
	}
}

// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/syncthing/syncthing/internal/gen/dbproto"
	"github.com/syncthing/syncthing/lib/protocol"
)

type ObservedFolder struct {
	Time             time.Time `json:"time"`
	Label            string    `json:"label"`
	ReceiveEncrypted bool      `json:"receiveEncrypted"`
	RemoteEncrypted  bool      `json:"remoteEncrypted"`
}

func (o *ObservedFolder) toWire() *dbproto.ObservedFolder {
	return &dbproto.ObservedFolder{
		Time:             timestamppb.New(o.Time),
		Label:            o.Label,
		ReceiveEncrypted: o.ReceiveEncrypted,
		RemoteEncrypted:  o.RemoteEncrypted,
	}
}

func (o *ObservedFolder) fromWire(w *dbproto.ObservedFolder) {
	o.Time = w.GetTime().AsTime()
	o.Label = w.GetLabel()
	o.ReceiveEncrypted = w.GetReceiveEncrypted()
	o.RemoteEncrypted = w.GetRemoteEncrypted()
}

type ObservedDevice struct {
	Time    time.Time `json:"time"`
	Name    string    `json:"name"`
	Address string    `json:"address"`
}

func (o *ObservedDevice) fromWire(w *dbproto.ObservedDevice) {
	o.Time = w.GetTime().AsTime()
	o.Name = w.GetName()
	o.Address = w.GetAddress()
}

func (db *Lowlevel) AddOrUpdatePendingDevice(device protocol.DeviceID, name, address string) error {
	key := db.keyer.GeneratePendingDeviceKey(nil, device[:])
	od := &dbproto.ObservedDevice{
		Time:    timestamppb.New(time.Now().Truncate(time.Second)),
		Name:    name,
		Address: address,
	}
	return db.Put(key, mustMarshal(od))
}

func (db *Lowlevel) RemovePendingDevice(device protocol.DeviceID) error {
	key := db.keyer.GeneratePendingDeviceKey(nil, device[:])
	return db.Delete(key)
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
		var protoD dbproto.ObservedDevice
		var od ObservedDevice
		if err != nil {
			goto deleteKey
		}
		if err = proto.Unmarshal(iter.Value(), &protoD); err != nil {
			goto deleteKey
		}
		od.fromWire(&protoD)
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

func (db *Lowlevel) AddOrUpdatePendingFolder(id string, of ObservedFolder, device protocol.DeviceID) error {
	key, err := db.keyer.GeneratePendingFolderKey(nil, device[:], []byte(id))
	if err != nil {
		return err
	}
	return db.Put(key, mustMarshal(of.toWire()))
}

// RemovePendingFolderForDevice removes entries for specific folder / device combinations.
func (db *Lowlevel) RemovePendingFolderForDevice(id string, device protocol.DeviceID) error {
	key, err := db.keyer.GeneratePendingFolderKey(nil, device[:], []byte(id))
	if err != nil {
		return err
	}
	return db.Delete(key)
}

// RemovePendingFolder removes all entries matching a specific folder ID.
func (db *Lowlevel) RemovePendingFolder(id string) error {
	iter, err := db.NewPrefixIterator([]byte{KeyTypePendingFolder})
	if err != nil {
		return fmt.Errorf("creating iterator: %w", err)
	}
	defer iter.Release()
	var iterErr error
	for iter.Next() {
		if id != string(db.keyer.FolderFromPendingFolderKey(iter.Key())) {
			continue
		}
		if err = db.Delete(iter.Key()); err != nil {
			if iterErr != nil {
				l.Debugf("Repeat error removing pending folder: %v", err)
			} else {
				iterErr = err
			}
		}
	}
	return iterErr
}

// Consolidated information about a pending folder
type PendingFolder struct {
	OfferedBy map[protocol.DeviceID]ObservedFolder `json:"offeredBy"`
}

func (db *Lowlevel) PendingFolders() (map[string]PendingFolder, error) {
	return db.PendingFoldersForDevice(protocol.EmptyDeviceID)
}

// PendingFoldersForDevice enumerates only entries matching the given device ID, unless it
// is EmptyDeviceID.  Invalid ones are dropped from the database after a info log
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
		var protoF dbproto.ObservedFolder
		var of ObservedFolder
		var folderID string
		if !ok || err != nil {
			goto deleteKey
		}
		if folderID = string(db.keyer.FolderFromPendingFolderKey(iter.Key())); len(folderID) < 1 {
			goto deleteKey
		}
		if err = proto.Unmarshal(iter.Value(), &protoF); err != nil {
			goto deleteKey
		}
		if _, ok := res[folderID]; !ok {
			res[folderID] = PendingFolder{
				OfferedBy: map[protocol.DeviceID]ObservedFolder{},
			}
		}
		of.fromWire(&protoF)
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

// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/syncthing/syncthing/internal/gen/dbproto"
	"github.com/syncthing/syncthing/lib/protocol"
)

type ObservedDB struct {
	kv KV
}

func NewObservedDB(kv KV) *ObservedDB {
	return &ObservedDB{kv: kv}
}

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

func (db *ObservedDB) AddOrUpdatePendingDevice(device protocol.DeviceID, name, address string) error {
	key := "device/" + device.String()
	od := &dbproto.ObservedDevice{
		Time:    timestamppb.New(time.Now().Truncate(time.Second)),
		Name:    name,
		Address: address,
	}
	return db.kv.PutKV(key, mustMarshal(od))
}

func (db *ObservedDB) RemovePendingDevice(device protocol.DeviceID) error {
	key := "device/" + device.String()
	return db.kv.DeleteKV(key)
}

// PendingDevices enumerates all entries.  Invalid ones are dropped from the database
// after a warning log message, as a side-effect.
func (db *ObservedDB) PendingDevices() (map[protocol.DeviceID]ObservedDevice, error) {
	res := make(map[protocol.DeviceID]ObservedDevice)
	it, errFn := db.kv.PrefixKV("device/")
	for kv := range it {
		_, keyDev, ok := strings.Cut(kv.Key, "/")
		if !ok {
			if err := db.kv.DeleteKV(kv.Key); err != nil {
				return nil, fmt.Errorf("delete invalid pending device: %w", err)
			}
			continue
		}

		deviceID, err := protocol.DeviceIDFromString(keyDev)
		var protoD dbproto.ObservedDevice
		var od ObservedDevice
		if err != nil {
			goto deleteKey
		}
		if err = proto.Unmarshal(kv.Value, &protoD); err != nil {
			goto deleteKey
		}
		od.fromWire(&protoD)
		res[deviceID] = od
		continue
	deleteKey:
		// Deleting invalid entries is the only possible "repair" measure and
		// appropriate for the importance of pending entries.  They will come back
		// soon if still relevant.
		if err := db.kv.DeleteKV(kv.Key); err != nil {
			return nil, fmt.Errorf("delete invalid pending device: %w", err)
		}
	}
	return res, errFn()
}

func (db *ObservedDB) AddOrUpdatePendingFolder(id string, of ObservedFolder, device protocol.DeviceID) error {
	key := "folder/" + device.String() + "/" + id
	return db.kv.PutKV(key, mustMarshal(of.toWire()))
}

// RemovePendingFolderForDevice removes entries for specific folder / device combinations.
func (db *ObservedDB) RemovePendingFolderForDevice(id string, device protocol.DeviceID) error {
	key := "folder/" + device.String() + "/" + id
	return db.kv.DeleteKV(key)
}

// RemovePendingFolder removes all entries matching a specific folder ID.
func (db *ObservedDB) RemovePendingFolder(id string) error {
	it, errFn := db.kv.PrefixKV("folder/")
	for kv := range it {
		parts := strings.Split(kv.Key, "/")
		if len(parts) != 3 || parts[2] != id {
			continue
		}
		if err := db.kv.DeleteKV(kv.Key); err != nil {
			return fmt.Errorf("delete pending folder: %w", err)
		}
	}
	return errFn()
}

// Consolidated information about a pending folder
type PendingFolder struct {
	OfferedBy map[protocol.DeviceID]ObservedFolder `json:"offeredBy"`
}

func (db *ObservedDB) PendingFolders() (map[string]PendingFolder, error) {
	return db.PendingFoldersForDevice(protocol.EmptyDeviceID)
}

// PendingFoldersForDevice enumerates only entries matching the given device ID, unless it
// is EmptyDeviceID.  Invalid ones are dropped from the database after a info log
// message, as a side-effect.
func (db *ObservedDB) PendingFoldersForDevice(device protocol.DeviceID) (map[string]PendingFolder, error) {
	prefix := "folder/"
	if device != protocol.EmptyDeviceID {
		prefix += device.String() + "/"
	}
	res := make(map[string]PendingFolder)
	it, errFn := db.kv.PrefixKV(prefix)
	for kv := range it {
		parts := strings.Split(kv.Key, "/")
		if len(parts) != 3 {
			continue
		}
		keyDev := parts[1]
		deviceID, err := protocol.DeviceIDFromString(keyDev)
		var protoF dbproto.ObservedFolder
		var of ObservedFolder
		var folderID string
		if err != nil {
			goto deleteKey
		}
		if folderID = parts[2]; len(folderID) < 1 {
			goto deleteKey
		}
		if err = proto.Unmarshal(kv.Value, &protoF); err != nil {
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
		if err := db.kv.DeleteKV(kv.Key); err != nil {
			return nil, fmt.Errorf("delete invalid pending folder: %w", err)
		}
	}
	return res, errFn()
}

func mustMarshal(m proto.Message) []byte {
	bs, err := proto.Marshal(m)
	if err != nil {
		panic(err)
	}
	return bs
}

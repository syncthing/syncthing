// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"

	"github.com/syndtr/goleveldb/leveldb"
)

// ConvertKeyFormat converts from the v0.11 to the v0.12 database format, to
// avoid having to do rescan. The change is in the key format for folder
// labels, so we basically just iterate over the database rewriting keys as
// necessary and then write out the folder ID mapping at the end.
func ConvertKeyFormat(from, to *leveldb.DB) error {
	l.Infoln("Converting database key format")
	files, globals, unchanged := 0, 0, 0

	dbi := newDBInstance(to)
	i := from.NewIterator(nil, nil)
	for i.Next() {
		key := i.Key()
		switch key[0] {
		case KeyTypeDevice:
			newKey := dbi.deviceKey(oldDeviceKeyFolder(key), oldDeviceKeyDevice(key), oldDeviceKeyName(key))
			if err := to.Put(newKey, i.Value(), nil); err != nil {
				return err
			}
			files++
		case KeyTypeGlobal:
			newKey := dbi.globalKey(oldGlobalKeyFolder(key), oldGlobalKeyName(key))
			if err := to.Put(newKey, i.Value(), nil); err != nil {
				return err
			}
			globals++
		default:
			if err := to.Put(key, i.Value(), nil); err != nil {
				return err
			}
			unchanged++
		}
	}

	l.Infof("Converted %d files, %d globals (%d unchanged).", files, globals, unchanged)

	return nil
}

func oldDeviceKeyFolder(key []byte) []byte {
	folder := key[1 : 1+64]
	izero := bytes.IndexByte(folder, 0)
	if izero < 0 {
		return folder
	}
	return folder[:izero]
}

func oldDeviceKeyDevice(key []byte) []byte {
	return key[1+64 : 1+64+32]
}

func oldDeviceKeyName(key []byte) []byte {
	return key[1+64+32:]
}

func oldGlobalKeyName(key []byte) []byte {
	return key[1+64:]
}

func oldGlobalKeyFolder(key []byte) []byte {
	folder := key[1 : 1+64]
	izero := bytes.IndexByte(folder, 0)
	if izero < 0 {
		return folder
	}
	return folder[:izero]
}

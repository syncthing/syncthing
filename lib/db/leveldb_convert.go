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

// convertKeyFormat converts from the v0.12 to the v0.13 database format, to
// avoid having to do rescan. The change is in the key format for folder
// labels, so we basically just iterate over the database rewriting keys as
// necessary.
func convertKeyFormat(from, to *leveldb.DB) error {
	l.Infoln("Converting database key format")
	blocks, files, globals, unchanged := 0, 0, 0, 0

	dbi := newDBInstance(to)
	i := from.NewIterator(nil, nil)
	for i.Next() {
		key := i.Key()
		switch key[0] {
		case KeyTypeBlock:
			folder, file := oldFromBlockKey(key)
			folderIdx := dbi.folderIdx.ID([]byte(folder))
			hash := key[1+64:]
			newKey := blockKeyInto(nil, hash, folderIdx, file)
			if err := to.Put(newKey, i.Value(), nil); err != nil {
				return err
			}
			blocks++

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

		case KeyTypeVirtualMtime:
			// Cannot be converted, we drop it instead :(

		default:
			if err := to.Put(key, i.Value(), nil); err != nil {
				return err
			}
			unchanged++
		}
	}

	l.Infof("Converted %d blocks, %d files, %d globals (%d unchanged).", blocks, files, globals, unchanged)

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

func oldFromBlockKey(data []byte) (string, string) {
	if len(data) < 1+64+32+1 {
		panic("Incorrect key length")
	}
	if data[0] != KeyTypeBlock {
		panic("Incorrect key type")
	}

	file := string(data[1+64+32:])

	slice := data[1 : 1+64]
	izero := bytes.IndexByte(slice, 0)
	if izero > -1 {
		return string(slice[:izero]), file
	}
	return string(slice), file
}

// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"encoding/binary"
	"fmt"
	"log"

	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syndtr/goleveldb/leveldb"
)

func dump(ldb *leveldb.DB) {
	it := ldb.NewIterator(nil, nil)
	var dev protocol.DeviceID
	for it.Next() {
		key := it.Key()
		switch key[0] {
		case db.KeyTypeDevice:
			folder := nulString(key[1 : 1+64])
			devBytes := key[1+64 : 1+64+32]
			name := nulString(key[1+64+32:])
			copy(dev[:], devBytes)
			fmt.Printf("[device] F:%q N:%q D:%v\n", folder, name, dev)

			var f protocol.FileInfo
			err := f.UnmarshalXDR(it.Value())
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("  N:%q\n  F:%#o\n  M:%d\n  V:%v\n  S:%d\n  B:%d\n", f.Name, f.Flags, f.Modified, f.Version, f.Size(), len(f.Blocks))

		case db.KeyTypeGlobal:
			folder := nulString(key[1 : 1+64])
			name := nulString(key[1+64:])
			var flv db.VersionList
			flv.UnmarshalXDR(it.Value())
			fmt.Printf("[global] F:%q N:%q V: %s\n", folder, name, flv)

		case db.KeyTypeBlock:
			folder := nulString(key[1 : 1+64])
			hash := key[1+64 : 1+64+32]
			name := nulString(key[1+64+32:])
			fmt.Printf("[block] F:%q H:%x N:%q I:%d\n", folder, hash, name, binary.BigEndian.Uint32(it.Value()))

		case db.KeyTypeDeviceStatistic:
			fmt.Printf("[dstat]\n  %x\n  %x\n", it.Key(), it.Value())

		case db.KeyTypeFolderStatistic:
			fmt.Printf("[fstat]\n  %x\n  %x\n", it.Key(), it.Value())

		case db.KeyTypeVirtualMtime:
			fmt.Printf("[mtime]\n  %x\n  %x\n", it.Key(), it.Value())

		default:
			fmt.Printf("[???]\n  %x\n  %x\n", it.Key(), it.Value())
		}
	}
}

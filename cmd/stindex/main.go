// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/db"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)

	flag.Parse()

	ldb, err := leveldb.OpenFile(flag.Arg(0), &opt.Options{
		ErrorIfMissing:         true,
		Strict:                 opt.StrictAll,
		OpenFilesCacheCapacity: 100,
	})
	if err != nil {
		log.Fatal(err)
	}

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
			fmt.Printf("[global] F:%q N:%q V:%x\n", folder, name, it.Value())

		case db.KeyTypeBlock:
			folder := nulString(key[1 : 1+64])
			hash := key[1+64 : 1+64+32]
			name := nulString(key[1+64+32:])
			fmt.Printf("[block] F:%q H:%x N:%q I:%d\n", folder, hash, name, binary.BigEndian.Uint32(it.Value()))

		case db.KeyTypeDeviceStatistic:
			fmt.Printf("[dstat]\n  %x\n  %x\n", it.Key(), it.Value())

		case db.KeyTypeFolderStatistic:
			fmt.Printf("[fstat]\n  %x\n  %x\n", it.Key(), it.Value())

		default:
			fmt.Printf("[???]\n  %x\n  %x\n", it.Key(), it.Value())
		}
	}
}

func nulString(bs []byte) string {
	for i := range bs {
		if bs[i] == 0 {
			return string(bs[:i])
		}
	}
	return string(bs)
}

// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/urfave/cli"

	"github.com/syncthing/syncthing/lib/db"
)

func indexDumpSize(*cli.Context) error {
	type sizedElement struct {
		key  string
		size int
	}

	ldb, err := getDB()
	if err != nil {
		return err
	}

	it, err := ldb.NewPrefixIterator(nil)
	if err != nil {
		return err
	}

	var elems []sizedElement
	for it.Next() {
		var ele sizedElement

		key := it.Key()
		switch key[0] {
		case db.KeyTypeDevice:
			folder := binary.BigEndian.Uint32(key[1:])
			device := binary.BigEndian.Uint32(key[1+4:])
			name := nulString(key[1+4+4:])
			ele.key = fmt.Sprintf("DEVICE:%d:%d:%s", folder, device, name)

		case db.KeyTypeGlobal:
			folder := binary.BigEndian.Uint32(key[1:])
			name := nulString(key[1+4:])
			ele.key = fmt.Sprintf("GLOBAL:%d:%s", folder, name)

		case db.KeyTypeBlock:
			folder := binary.BigEndian.Uint32(key[1:])
			hash := key[1+4 : 1+4+32]
			name := nulString(key[1+4+32:])
			ele.key = fmt.Sprintf("BLOCK:%d:%x:%s", folder, hash, name)

		case db.KeyTypeDeviceStatistic:
			ele.key = fmt.Sprintf("DEVICESTATS:%s", key[1:])

		case db.KeyTypeFolderStatistic:
			ele.key = fmt.Sprintf("FOLDERSTATS:%s", key[1:])

		case db.KeyTypeVirtualMtime:
			ele.key = fmt.Sprintf("MTIME:%s", key[1:])

		case db.KeyTypeFolderIdx:
			id := binary.BigEndian.Uint32(key[1:])
			ele.key = fmt.Sprintf("FOLDERIDX:%d", id)

		case db.KeyTypeDeviceIdx:
			id := binary.BigEndian.Uint32(key[1:])
			ele.key = fmt.Sprintf("DEVICEIDX:%d", id)

		default:
			ele.key = fmt.Sprintf("UNKNOWN:%x", key)
		}
		ele.size = len(it.Value())
		elems = append(elems, ele)
	}

	sort.Slice(elems, func(i, j int) bool {
		return elems[i].size > elems[j].size
	})
	for _, ele := range elems {
		fmt.Println(ele.key, ele.size)
	}

	return nil
}

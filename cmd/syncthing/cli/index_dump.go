// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/protocol"
)

func indexDump() error {
	ldb, err := getDB()
	if err != nil {
		return err
	}
	it, err := ldb.NewPrefixIterator(nil)
	if err != nil {
		return err
	}
	for it.Next() {
		key := it.Key()
		switch key[0] {
		case db.KeyTypeDevice:
			folder := binary.BigEndian.Uint32(key[1:])
			device := binary.BigEndian.Uint32(key[1+4:])
			name := nulString(key[1+4+4:])
			fmt.Printf("[device] F:%d D:%d N:%q", folder, device, name)

			var f protocol.FileInfo
			err := f.Unmarshal(it.Value())
			if err != nil {
				return err
			}
			fmt.Printf(" V:%v\n", f)

		case db.KeyTypeGlobal:
			folder := binary.BigEndian.Uint32(key[1:])
			name := nulString(key[1+4:])
			var flv db.VersionList
			flv.Unmarshal(it.Value())
			fmt.Printf("[global] F:%d N:%q V:%s\n", folder, name, flv)

		case db.KeyTypeBlock:
			folder := binary.BigEndian.Uint32(key[1:])
			hash := key[1+4 : 1+4+32]
			name := nulString(key[1+4+32:])
			fmt.Printf("[block] F:%d H:%x N:%q I:%d\n", folder, hash, name, binary.BigEndian.Uint32(it.Value()))

		case db.KeyTypeDeviceStatistic:
			fmt.Printf("[dstat] K:%x V:%x\n", key, it.Value())

		case db.KeyTypeFolderStatistic:
			fmt.Printf("[fstat] K:%x V:%x\n", key, it.Value())

		case db.KeyTypeVirtualMtime:
			folder := binary.BigEndian.Uint32(key[1:])
			name := nulString(key[1+4:])
			val := it.Value()
			var realTime, virtualTime time.Time
			realTime.UnmarshalBinary(val[:len(val)/2])
			virtualTime.UnmarshalBinary(val[len(val)/2:])
			fmt.Printf("[mtime] F:%d N:%q R:%v V:%v\n", folder, name, realTime, virtualTime)

		case db.KeyTypeFolderIdx:
			key := binary.BigEndian.Uint32(key[1:])
			fmt.Printf("[folderidx] K:%d V:%q\n", key, it.Value())

		case db.KeyTypeDeviceIdx:
			key := binary.BigEndian.Uint32(key[1:])
			val := it.Value()
			device := "<nil>"
			if len(val) > 0 {
				dev, err := protocol.DeviceIDFromBytes(val)
				if err != nil {
					device = fmt.Sprintf("<invalid %d bytes>", len(val))
				} else {
					device = dev.String()
				}
			}
			fmt.Printf("[deviceidx] K:%d V:%s\n", key, device)

		case db.KeyTypeIndexID:
			device := binary.BigEndian.Uint32(key[1:])
			folder := binary.BigEndian.Uint32(key[5:])
			fmt.Printf("[indexid] D:%d F:%d I:%x\n", device, folder, it.Value())

		case db.KeyTypeFolderMeta:
			folder := binary.BigEndian.Uint32(key[1:])
			fmt.Printf("[foldermeta] F:%d", folder)
			var cs db.CountsSet
			if err := cs.Unmarshal(it.Value()); err != nil {
				fmt.Printf(" (invalid)\n")
			} else {
				fmt.Printf(" V:%v\n", cs)
			}

		case db.KeyTypeMiscData:
			fmt.Printf("[miscdata] K:%q V:%q\n", key[1:], it.Value())

		case db.KeyTypeSequence:
			folder := binary.BigEndian.Uint32(key[1:])
			seq := binary.BigEndian.Uint64(key[5:])
			fmt.Printf("[sequence] F:%d S:%d V:%q\n", folder, seq, it.Value())

		case db.KeyTypeNeed:
			folder := binary.BigEndian.Uint32(key[1:])
			file := string(key[5:])
			fmt.Printf("[need] F:%d V:%q\n", folder, file)

		case db.KeyTypeBlockList:
			fmt.Printf("[blocklist] H:%x\n", key[1:])

		case db.KeyTypeBlockListMap:
			folder := binary.BigEndian.Uint32(key[1:])
			hash := key[5:37]
			fileName := string(key[37:])
			fmt.Printf("[blocklistmap] F:%d H:%x N:%s\n", folder, hash, fileName)

		case db.KeyTypeVersion:
			fmt.Printf("[version] H:%x", key[1:])
			var v protocol.Vector
			err := v.Unmarshal(it.Value())
			if err != nil {
				fmt.Printf(" (invalid)\n")
			} else {
				fmt.Printf(" V:%v\n", v)
			}

		case db.KeyTypePendingFolder:
			device := binary.BigEndian.Uint32(key[1:])
			folder := string(key[5:])
			var of db.ObservedFolder
			of.Unmarshal(it.Value())
			fmt.Printf("[pendingFolder] D:%d F:%s V:%v\n", device, folder, of)

		case db.KeyTypePendingDevice:
			device := "<invalid>"
			dev, err := protocol.DeviceIDFromBytes(key[1:])
			if err == nil {
				device = dev.String()
			}
			var od db.ObservedDevice
			od.Unmarshal(it.Value())
			fmt.Printf("[pendingDevice] D:%v V:%v\n", device, od)

		default:
			fmt.Printf("[??? %d]\n  %x\n  %x\n", key[0], key, it.Value())
		}
	}
	return nil
}

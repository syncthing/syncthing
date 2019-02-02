// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/protocol"
)

type fileInfoKey struct {
	folder uint32
	device uint32
	name   string
}

type globalKey struct {
	folder uint32
	name   string
}

type sequenceKey struct {
	folder   uint32
	sequence uint64
}

func idxck(ldb *db.Lowlevel) (success bool) {
	folders := make(map[uint32]string)
	devices := make(map[uint32]string)
	deviceToIDs := make(map[string]uint32)
	fileInfos := make(map[fileInfoKey]protocol.FileInfo)
	globals := make(map[globalKey]db.VersionList)
	sequences := make(map[sequenceKey]string)
	needs := make(map[globalKey]struct{})
	var localDeviceKey uint32
	success = true

	it := ldb.NewIterator(nil, nil)
	for it.Next() {
		key := it.Key()
		switch key[0] {
		case db.KeyTypeDevice:
			folder := binary.BigEndian.Uint32(key[1:])
			device := binary.BigEndian.Uint32(key[1+4:])
			name := nulString(key[1+4+4:])

			var f protocol.FileInfo
			err := f.Unmarshal(it.Value())
			if err != nil {
				fmt.Println("Unable to unmarshal FileInfo:", err)
				success = false
				continue
			}

			fileInfos[fileInfoKey{folder, device, name}] = f

		case db.KeyTypeGlobal:
			folder := binary.BigEndian.Uint32(key[1:])
			name := nulString(key[1+4:])
			var flv db.VersionList
			if err := flv.Unmarshal(it.Value()); err != nil {
				fmt.Println("Unable to unmarshal VersionList:", err)
				success = false
				continue
			}
			globals[globalKey{folder, name}] = flv

		case db.KeyTypeFolderIdx:
			key := binary.BigEndian.Uint32(it.Key()[1:])
			folders[key] = string(it.Value())

		case db.KeyTypeDeviceIdx:
			key := binary.BigEndian.Uint32(it.Key()[1:])
			devices[key] = string(it.Value())
			deviceToIDs[string(it.Value())] = key
			if bytes.Equal(it.Value(), protocol.LocalDeviceID[:]) {
				localDeviceKey = key
			}

		case db.KeyTypeSequence:
			folder := binary.BigEndian.Uint32(key[1:])
			seq := binary.BigEndian.Uint64(key[5:])
			val := it.Value()
			sequences[sequenceKey{folder, seq}] = string(val[9:])

		case db.KeyTypeNeed:
			folder := binary.BigEndian.Uint32(key[1:])
			name := nulString(key[1+4:])
			needs[globalKey{folder, name}] = struct{}{}
		}
	}

	if localDeviceKey == 0 {
		fmt.Println("Missing key for local device in device index (bailing out)")
		success = false
		return
	}

	for fk, fi := range fileInfos {
		if fk.name != fi.Name {
			fmt.Printf("Mismatching FileInfo name, %q (key) != %q (actual)\n", fk.name, fi.Name)
			success = false
		}

		folder := folders[fk.folder]
		if folder == "" {
			fmt.Printf("Unknown folder ID %d for FileInfo %q\n", fk.folder, fk.name)
			success = false
			continue
		}
		if devices[fk.device] == "" {
			fmt.Printf("Unknown device ID %d for FileInfo %q, folder %q\n", fk.folder, fk.name, folder)
			success = false
		}

		if fk.device == localDeviceKey {
			name, ok := sequences[sequenceKey{fk.folder, uint64(fi.Sequence)}]
			if !ok {
				fmt.Printf("Sequence entry missing for FileInfo %q, folder %q, seq %d\n", fi.Name, folder, fi.Sequence)
				success = false
				continue
			}
			if name != fi.Name {
				fmt.Printf("Sequence entry refers to wrong name, %q (seq) != %q (FileInfo), folder %q, seq %d\n", name, fi.Name, folder, fi.Sequence)
				success = false
			}
		}
	}

	for gk, vl := range globals {
		folder := folders[gk.folder]
		if folder == "" {
			fmt.Printf("Unknown folder ID %d for VersionList %q\n", gk.folder, gk.name)
			success = false
		}
		for i, fv := range vl.Versions {
			dev, ok := deviceToIDs[string(fv.Device)]
			if !ok {
				fmt.Printf("VersionList %q, folder %q refers to unknown device %q\n", gk.name, folder, fv.Device)
				success = false
			}
			fi, ok := fileInfos[fileInfoKey{gk.folder, dev, gk.name}]
			if !ok {
				fmt.Printf("VersionList %q, folder %q, entry %d refers to unknown FileInfo\n", gk.name, folder, i)
				success = false
			}
			if !fi.Version.Equal(fv.Version) {
				fmt.Printf("VersionList %q, folder %q, entry %d, FileInfo version mismatch, %v (VersionList) != %v (FileInfo)\n", gk.name, folder, i, fv.Version, fi.Version)
				success = false
			}
			if fi.IsInvalid() != fv.Invalid {
				fmt.Printf("VersionList %q, folder %q, entry %d, FileInfo invalid mismatch, %v (VersionList) != %v (FileInfo)\n", gk.name, folder, i, fv.Invalid, fi.IsInvalid())
				success = false
			}
		}

		// If we need this file we should have a need entry for it. False
		// positives from needsLocally for deleted files, where we might
		// legitimately lack an entry if we never had it, and ignored files.
		if needsLocally(vl) {
			_, ok := needs[gk]
			if !ok {
				dev := deviceToIDs[string(vl.Versions[0].Device)]
				fi := fileInfos[fileInfoKey{gk.folder, dev, gk.name}]
				if !fi.IsDeleted() && !fi.IsIgnored() {
					fmt.Printf("Missing need entry for needed file %q, folder %q\n", gk.name, folder)
				}
			}
		}
	}

	seenSeq := make(map[fileInfoKey]uint64)
	for sk, name := range sequences {
		folder := folders[sk.folder]
		if folder == "" {
			fmt.Printf("Unknown folder ID %d for sequence entry %d, %q\n", sk.folder, sk.sequence, name)
			success = false
			continue
		}

		if prev, ok := seenSeq[fileInfoKey{folder: sk.folder, name: name}]; ok {
			fmt.Printf("Duplicate sequence entry for %q, folder %q, seq %d (prev %d)\n", name, folder, sk.sequence, prev)
			success = false
		}
		seenSeq[fileInfoKey{folder: sk.folder, name: name}] = sk.sequence

		fi, ok := fileInfos[fileInfoKey{sk.folder, localDeviceKey, name}]
		if !ok {
			fmt.Printf("Missing FileInfo for sequence entry %d, folder %q, %q\n", sk.sequence, folder, name)
			success = false
			continue
		}
		if fi.Sequence != int64(sk.sequence) {
			fmt.Printf("Sequence mismatch for %q, folder %q, %d (key) != %d (FileInfo)\n", name, folder, sk.sequence, fi.Sequence)
			success = false
		}
	}

	for nk := range needs {
		folder := folders[nk.folder]
		if folder == "" {
			fmt.Printf("Unknown folder ID %d for need entry %q\n", nk.folder, nk.name)
			success = false
			continue
		}

		vl, ok := globals[nk]
		if !ok {
			fmt.Printf("Missing global for need entry %q, folder %q\n", nk.name, folder)
			success = false
			continue
		}

		if !needsLocally(vl) {
			fmt.Printf("Need entry for file we don't need, %q, folder %q\n", nk.name, folder)
			success = false
		}
	}

	return
}

func needsLocally(vl db.VersionList) bool {
	var lv *protocol.Vector
	for _, fv := range vl.Versions {
		if bytes.Equal(fv.Device, protocol.LocalDeviceID[:]) {
			lv = &fv.Version
			break
		}
	}
	if lv == nil {
		return true // proviosinally, it looks like we need the file
	}
	return !lv.GreaterEqual(vl.Versions[0].Version)
}

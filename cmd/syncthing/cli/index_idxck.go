// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"

	"github.com/syncthing/syncthing/internal/gen/bep"
	"github.com/syncthing/syncthing/internal/gen/dbproto"
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

func indexCheck() (err error) {
	ldb, err := getDB()
	if err != nil {
		return err
	}

	folders := make(map[uint32]string)
	devices := make(map[uint32]string)
	deviceToIDs := make(map[string]uint32)
	fileInfos := make(map[fileInfoKey]*bep.FileInfo)
	globals := make(map[globalKey]*dbproto.VersionList)
	sequences := make(map[sequenceKey]string)
	needs := make(map[globalKey]struct{})
	blocklists := make(map[string]struct{})
	versions := make(map[string]*bep.Vector)
	usedBlocklists := make(map[string]struct{})
	usedVersions := make(map[string]struct{})
	var localDeviceKey uint32
	success := true
	defer func() {
		if err == nil {
			if success {
				fmt.Println("Index check completed successfully.")
			} else {
				err = errors.New("Inconsistencies found in the index")
			}
		}
	}()

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

			var f bep.FileInfo
			err := proto.Unmarshal(it.Value(), &f)
			if err != nil {
				fmt.Println("Unable to unmarshal FileInfo:", err)
				success = false
				continue
			}

			fileInfos[fileInfoKey{folder, device, name}] = &f

		case db.KeyTypeGlobal:
			folder := binary.BigEndian.Uint32(key[1:])
			name := nulString(key[1+4:])
			var flv dbproto.VersionList
			if err := proto.Unmarshal(it.Value(), &flv); err != nil {
				fmt.Println("Unable to unmarshal VersionList:", err)
				success = false
				continue
			}
			globals[globalKey{folder, name}] = &flv

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

		case db.KeyTypeBlockList:
			hash := string(key[1:])
			blocklists[hash] = struct{}{}

		case db.KeyTypeVersion:
			hash := string(key[1:])
			var v bep.Vector
			if err := proto.Unmarshal(it.Value(), &v); err != nil {
				fmt.Println("Unable to unmarshal Vector:", err)
				success = false
				continue
			}
			versions[hash] = &v
		}
	}

	if localDeviceKey == 0 {
		fmt.Println("Missing key for local device in device index (bailing out)")
		success = false
		return
	}

	var missingSeq []sequenceKey
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
			sk := sequenceKey{fk.folder, uint64(fi.Sequence)}
			name, ok := sequences[sk]
			if !ok {
				fmt.Printf("Sequence entry missing for FileInfo %q, folder %q, seq %d\n", fi.Name, folder, fi.Sequence)
				missingSeq = append(missingSeq, sk)
				success = false
				continue
			}
			if name != fi.Name {
				fmt.Printf("Sequence entry refers to wrong name, %q (seq) != %q (FileInfo), folder %q, seq %d\n", name, fi.Name, folder, fi.Sequence)
				success = false
			}
		}

		if len(fi.Blocks) == 0 && len(fi.BlocksHash) != 0 {
			key := string(fi.BlocksHash)
			if _, ok := blocklists[key]; !ok {
				fmt.Printf("Missing block list for file %q, block list hash %x\n", fi.Name, fi.BlocksHash)
				success = false
			} else {
				usedBlocklists[key] = struct{}{}
			}
		}

		if fi.VersionHash != nil {
			key := string(fi.VersionHash)
			if _, ok := versions[key]; !ok {
				fmt.Printf("Missing version vector for file %q, version hash %x\n", fi.Name, fi.VersionHash)
				success = false
			} else {
				usedVersions[key] = struct{}{}
			}
		}

		_, ok := globals[globalKey{fk.folder, fk.name}]
		if !ok {
			fmt.Printf("Missing global for file %q\n", fi.Name)
			success = false
			continue
		}
	}

	// Aggregate the ranges of missing sequence entries, print them

	sort.Slice(missingSeq, func(a, b int) bool {
		if missingSeq[a].folder != missingSeq[b].folder {
			return missingSeq[a].folder < missingSeq[b].folder
		}
		return missingSeq[a].sequence < missingSeq[b].sequence
	})

	var folder uint32
	var startSeq, prevSeq uint64
	for _, sk := range missingSeq {
		if folder != sk.folder || sk.sequence != prevSeq+1 {
			if folder != 0 {
				fmt.Printf("Folder %d missing %d sequence entries: #%d - #%d\n", folder, prevSeq-startSeq+1, startSeq, prevSeq)
			}
			startSeq = sk.sequence
			folder = sk.folder
		}
		prevSeq = sk.sequence
	}
	if folder != 0 {
		fmt.Printf("Folder %d missing %d sequence entries: #%d - #%d\n", folder, prevSeq-startSeq+1, startSeq, prevSeq)
	}

	for gk, vl := range globals {
		folder := folders[gk.folder]
		if folder == "" {
			fmt.Printf("Unknown folder ID %d for VersionList %q\n", gk.folder, gk.name)
			success = false
		}
		checkGlobal := func(i int, device []byte, version protocol.Vector, invalid, deleted bool) {
			dev, ok := deviceToIDs[string(device)]
			if !ok {
				fmt.Printf("VersionList %q, folder %q refers to unknown device %q\n", gk.name, folder, device)
				success = false
			}
			fi, ok := fileInfos[fileInfoKey{gk.folder, dev, gk.name}]
			if !ok {
				fmt.Printf("VersionList %q, folder %q, entry %d refers to unknown FileInfo\n", gk.name, folder, i)
				success = false
			}

			fiv := fi.Version
			if fi.VersionHash != nil {
				fiv = versions[string(fi.VersionHash)]
			}
			if !protocol.VectorFromWire(fiv).Equal(version) {
				fmt.Printf("VersionList %q, folder %q, entry %d, FileInfo version mismatch, %v (VersionList) != %v (FileInfo)\n", gk.name, folder, i, version, fi.Version)
				success = false
			}
			ffi := protocol.FileInfoFromDB(fi)
			if ffi.IsInvalid() != invalid {
				fmt.Printf("VersionList %q, folder %q, entry %d, FileInfo invalid mismatch, %v (VersionList) != %v (FileInfo)\n", gk.name, folder, i, invalid, ffi.IsInvalid())
				success = false
			}
			if ffi.IsDeleted() != deleted {
				fmt.Printf("VersionList %q, folder %q, entry %d, FileInfo deleted mismatch, %v (VersionList) != %v (FileInfo)\n", gk.name, folder, i, deleted, ffi.IsDeleted())
				success = false
			}
		}
		for i, fv := range vl.Versions {
			ver := protocol.VectorFromWire(fv.Version)
			for _, device := range fv.Devices {
				checkGlobal(i, device, ver, false, fv.Deleted)
			}
			for _, device := range fv.InvalidDevices {
				checkGlobal(i, device, ver, true, fv.Deleted)
			}
		}

		// If we need this file we should have a need entry for it. False
		// positives from needsLocally for deleted files, where we might
		// legitimately lack an entry if we never had it, and ignored files.
		if needsLocally(vl) {
			_, ok := needs[gk]
			if !ok {
				fv, _ := vlGetGlobal(vl)
				devB, _ := fvFirstDevice(fv)
				dev := deviceToIDs[string(devB)]
				fi := protocol.FileInfoFromDB(fileInfos[fileInfoKey{gk.folder, dev, gk.name}])
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

	if d := len(blocklists) - len(usedBlocklists); d > 0 {
		fmt.Printf("%d block list entries out of %d needs GC\n", d, len(blocklists))
	}
	if d := len(versions) - len(usedVersions); d > 0 {
		fmt.Printf("%d version entries out of %d needs GC\n", d, len(versions))
	}

	return nil
}

func needsLocally(vl *dbproto.VersionList) bool {
	gfv, gok := vlGetGlobal(vl)
	if !gok { // That's weird, but we hardly need something non-existent
		return false
	}
	fv, ok := vlGet(vl, protocol.LocalDeviceID[:])
	return db.Need(gfv, ok, protocol.VectorFromWire(fv.Version))
}

// Get returns a FileVersion that contains the given device and whether it has
// been found at all.
func vlGet(vl *dbproto.VersionList, device []byte) (*dbproto.FileVersion, bool) {
	_, i, _, ok := vlFindDevice(vl, device)
	if !ok {
		return &dbproto.FileVersion{}, false
	}
	return vl.Versions[i], true
}

// GetGlobal returns the current global FileVersion. The returned FileVersion
// may be invalid, if all FileVersions are invalid. Returns false only if
// VersionList is empty.
func vlGetGlobal(vl *dbproto.VersionList) (*dbproto.FileVersion, bool) {
	i := vlFindGlobal(vl)
	if i == -1 {
		return nil, false
	}
	return vl.Versions[i], true
}

// findGlobal returns the first version that isn't invalid, or if all versions are
// invalid just the first version (i.e. 0) or -1, if there's no versions at all.
func vlFindGlobal(vl *dbproto.VersionList) int {
	for i := range vl.Versions {
		if !fvIsInvalid(vl.Versions[i]) {
			return i
		}
	}
	if len(vl.Versions) == 0 {
		return -1
	}
	return 0
}

// findDevice returns whether the device is in InvalidVersions or Versions and
// in InvalidDevices or Devices (true for invalid), the positions in the version
// and device slices and whether it has been found at all.
func vlFindDevice(vl *dbproto.VersionList, device []byte) (bool, int, int, bool) {
	for i, v := range vl.Versions {
		if j := deviceIndex(v.Devices, device); j != -1 {
			return false, i, j, true
		}
		if j := deviceIndex(v.InvalidDevices, device); j != -1 {
			return true, i, j, true
		}
	}
	return false, -1, -1, false
}

func deviceIndex(devices [][]byte, device []byte) int {
	for i, dev := range devices {
		if bytes.Equal(device, dev) {
			return i
		}
	}
	return -1
}

func fvFirstDevice(fv *dbproto.FileVersion) ([]byte, bool) {
	if len(fv.Devices) != 0 {
		return fv.Devices[0], true
	}
	if len(fv.InvalidDevices) != 0 {
		return fv.InvalidDevices[0], true
	}
	return nil, false
}

func fvIsInvalid(fv *dbproto.FileVersion) bool {
	return fv == nil || len(fv.Devices) == 0
}

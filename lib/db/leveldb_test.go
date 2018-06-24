// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"testing"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestDeviceKey(t *testing.T) {
	fld := []byte("folder6789012345678901234567890123456789012345678901234567890123")
	dev := []byte("device67890123456789012345678901")
	name := []byte("name")

	db := OpenMemory()
	db.folderIdx.ID(fld)
	db.deviceIdx.ID(dev)

	key := db.deviceKey(fld, dev, name)

	fld2 := db.deviceKeyFolder(key)
	if !bytes.Equal(fld2, fld) {
		t.Errorf("wrong folder %q != %q", fld2, fld)
	}
	dev2 := db.deviceKeyDevice(key)
	if !bytes.Equal(dev2, dev) {
		t.Errorf("wrong device %q != %q", dev2, dev)
	}
	name2 := db.deviceKeyName(key)
	if !bytes.Equal(name2, name) {
		t.Errorf("wrong name %q != %q", name2, name)
	}
}

func TestGlobalKey(t *testing.T) {
	fld := []byte("folder6789012345678901234567890123456789012345678901234567890123")
	name := []byte("name")

	db := OpenMemory()
	db.folderIdx.ID(fld)

	key := db.globalKey(fld, name)

	fld2, ok := db.globalKeyFolder(key)
	if !ok {
		t.Error("should have been found")
	}
	if !bytes.Equal(fld2, fld) {
		t.Errorf("wrong folder %q != %q", fld2, fld)
	}
	name2 := db.globalKeyName(key)
	if !bytes.Equal(name2, name) {
		t.Errorf("wrong name %q != %q", name2, name)
	}

	_, ok = db.globalKeyFolder([]byte{1, 2, 3, 4, 5})
	if ok {
		t.Error("should not have been found")
	}
}

func TestDropIndexIDs(t *testing.T) {
	db := OpenMemory()

	d1 := []byte("device67890123456789012345678901")
	d2 := []byte("device12345678901234567890123456")

	// Set some index IDs

	db.setIndexID(protocol.LocalDeviceID[:], []byte("foo"), 1)
	db.setIndexID(protocol.LocalDeviceID[:], []byte("bar"), 2)
	db.setIndexID(d1, []byte("foo"), 3)
	db.setIndexID(d1, []byte("bar"), 4)
	db.setIndexID(d2, []byte("foo"), 5)
	db.setIndexID(d2, []byte("bar"), 6)

	// Verify them

	if db.getIndexID(protocol.LocalDeviceID[:], []byte("foo")) != 1 {
		t.Fatal("fail local 1")
	}
	if db.getIndexID(protocol.LocalDeviceID[:], []byte("bar")) != 2 {
		t.Fatal("fail local 2")
	}
	if db.getIndexID(d1, []byte("foo")) != 3 {
		t.Fatal("fail remote 1")
	}
	if db.getIndexID(d1, []byte("bar")) != 4 {
		t.Fatal("fail remote 2")
	}
	if db.getIndexID(d2, []byte("foo")) != 5 {
		t.Fatal("fail remote 3")
	}
	if db.getIndexID(d2, []byte("bar")) != 6 {
		t.Fatal("fail remote 4")
	}

	// Drop the local ones, verify only they got dropped

	db.DropLocalDeltaIndexIDs()

	if db.getIndexID(protocol.LocalDeviceID[:], []byte("foo")) != 0 {
		t.Fatal("fail local 1")
	}
	if db.getIndexID(protocol.LocalDeviceID[:], []byte("bar")) != 0 {
		t.Fatal("fail local 2")
	}
	if db.getIndexID(d1, []byte("foo")) != 3 {
		t.Fatal("fail remote 1")
	}
	if db.getIndexID(d1, []byte("bar")) != 4 {
		t.Fatal("fail remote 2")
	}
	if db.getIndexID(d2, []byte("foo")) != 5 {
		t.Fatal("fail remote 3")
	}
	if db.getIndexID(d2, []byte("bar")) != 6 {
		t.Fatal("fail remote 4")
	}

	// Set local ones again

	db.setIndexID(protocol.LocalDeviceID[:], []byte("foo"), 1)
	db.setIndexID(protocol.LocalDeviceID[:], []byte("bar"), 2)

	// Drop the remote ones, verify only they got dropped

	db.DropRemoteDeltaIndexIDs()

	if db.getIndexID(protocol.LocalDeviceID[:], []byte("foo")) != 1 {
		t.Fatal("fail local 1")
	}
	if db.getIndexID(protocol.LocalDeviceID[:], []byte("bar")) != 2 {
		t.Fatal("fail local 2")
	}
	if db.getIndexID(d1, []byte("foo")) != 0 {
		t.Fatal("fail remote 1")
	}
	if db.getIndexID(d1, []byte("bar")) != 0 {
		t.Fatal("fail remote 2")
	}
	if db.getIndexID(d2, []byte("foo")) != 0 {
		t.Fatal("fail remote 3")
	}
	if db.getIndexID(d2, []byte("bar")) != 0 {
		t.Fatal("fail remote 4")
	}
}

func TestIgnoredFiles(t *testing.T) {
	ldb, err := openJSONS("testdata/v0.14.48-ignoredfiles.db.jsons")
	if err != nil {
		t.Fatal(err)
	}
	db := newDBInstance(ldb, "<memory>")
	fs := NewFileSet("test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), db)

	// The contents of the database are like this:
	//
	// 	fs := NewFileSet("test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), db)
	// 	fs.Update(protocol.LocalDeviceID, []protocol.FileInfo{
	// 		{ // invalid (ignored) file
	// 			Name:    "foo",
	// 			Type:    protocol.FileInfoTypeFile,
	// 			Invalid: true,
	// 			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 1, Value: 1000}}},
	// 		},
	// 		{ // regular file
	// 			Name:    "bar",
	// 			Type:    protocol.FileInfoTypeFile,
	// 			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 1, Value: 1001}}},
	// 		},
	// 	})
	// 	fs.Update(protocol.DeviceID{42}, []protocol.FileInfo{
	// 		{ // invalid file
	// 			Name:    "baz",
	// 			Type:    protocol.FileInfoTypeFile,
	// 			Invalid: true,
	// 			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: 1000}}},
	// 		},
	// 		{ // regular file
	// 			Name:    "quux",
	// 			Type:    protocol.FileInfoTypeFile,
	// 			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: 1002}}},
	// 		},
	// 	})

	// Local files should have the "ignored" bit in addition to just being
	// generally invalid if we want to look at the simulation of that bit.

	fi, ok := fs.Get(protocol.LocalDeviceID, "foo")
	if !ok {
		t.Fatal("foo should exist")
	}
	if !fi.IsInvalid() {
		t.Error("foo should be invalid")
	}
	if !fi.IsIgnored() {
		t.Error("foo should be ignored")
	}

	fi, ok = fs.Get(protocol.LocalDeviceID, "bar")
	if !ok {
		t.Fatal("bar should exist")
	}
	if fi.IsInvalid() {
		t.Error("bar should not be invalid")
	}
	if fi.IsIgnored() {
		t.Error("bar should not be ignored")
	}

	// Remote files have the invalid bit as usual, and the IsInvalid() method
	// should pick this up too.

	fi, ok = fs.Get(protocol.DeviceID{42}, "baz")
	if !ok {
		t.Fatal("baz should exist")
	}
	if !fi.IsInvalid() {
		t.Error("baz should be invalid")
	}
	if !fi.IsInvalid() {
		t.Error("baz should be invalid")
	}

	fi, ok = fs.Get(protocol.DeviceID{42}, "quux")
	if !ok {
		t.Fatal("quux should exist")
	}
	if fi.IsInvalid() {
		t.Error("quux should not be invalid")
	}
	if fi.IsInvalid() {
		t.Error("quux should not be invalid")
	}
}

const myID = 1

var (
	remoteDevice0, remoteDevice1 protocol.DeviceID
	update0to3Folder             = "UpdateSchema0to3"
	invalid                      = "invalid"
	slashPrefixed                = "/notgood"
	haveUpdate0to3               map[protocol.DeviceID]fileList
)

func init() {
	remoteDevice0, _ = protocol.DeviceIDFromString("AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR")
	remoteDevice1, _ = protocol.DeviceIDFromString("I6KAH76-66SLLLB-5PFXSOA-UFJCDZC-YAOMLEK-CP2GB32-BV5RQST-3PSROAU")
	haveUpdate0to3 = map[protocol.DeviceID]fileList{
		protocol.LocalDeviceID: {
			protocol.FileInfo{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(1)},
			protocol.FileInfo{Name: slashPrefixed, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(1)},
		},
		remoteDevice0: {
			protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}, Blocks: genBlocks(2)},
			protocol.FileInfo{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(5), RawInvalid: true},
			protocol.FileInfo{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}, Blocks: genBlocks(7)},
		},
		remoteDevice1: {
			protocol.FileInfo{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(7)},
			protocol.FileInfo{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}, Blocks: genBlocks(5), RawInvalid: true},
			protocol.FileInfo{Name: invalid, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1004}}}, Blocks: genBlocks(5), RawInvalid: true},
		},
	}
}

func TestUpdate0to3(t *testing.T) {
	ldb, err := openJSONS("testdata/v0.14.45-update0to3.db.jsons")

	if err != nil {
		t.Fatal(err)
	}
	db := newDBInstance(ldb, "<memory>")

	folder := []byte(update0to3Folder)

	db.updateSchema0to1()

	if _, ok := db.getFile(db.deviceKey(folder, protocol.LocalDeviceID[:], []byte(slashPrefixed))); ok {
		t.Error("File prefixed by '/' was not removed during transition to schema 1")
	}

	if _, err := db.Get(db.globalKey(folder, []byte(invalid)), nil); err != nil {
		t.Error("Invalid file wasn't added to global list")
	}

	db.updateSchema1to2()

	found := false
	db.withHaveSequence(folder, 0, func(fi FileIntf) bool {
		f := fi.(protocol.FileInfo)
		l.Infoln(f)
		if found {
			t.Error("Unexpected additional file via sequence", f.FileName())
			return true
		}
		if e := haveUpdate0to3[protocol.LocalDeviceID][0]; f.IsEquivalent(e, true, true) {
			found = true
		} else {
			t.Errorf("Wrong file via sequence, got %v, expected %v", f, e)
		}
		return true
	})
	if !found {
		t.Error("Local file wasn't added to sequence bucket", err)
	}

	db.updateSchema2to3()

	need := map[string]protocol.FileInfo{
		haveUpdate0to3[remoteDevice0][0].Name: haveUpdate0to3[remoteDevice0][0],
		haveUpdate0to3[remoteDevice1][0].Name: haveUpdate0to3[remoteDevice1][0],
		haveUpdate0to3[remoteDevice0][2].Name: haveUpdate0to3[remoteDevice0][2],
	}
	db.withNeed(folder, protocol.LocalDeviceID[:], false, func(fi FileIntf) bool {
		e, ok := need[fi.FileName()]
		if !ok {
			t.Error("Got unexpected needed file:", fi.FileName())
		}
		f := fi.(protocol.FileInfo)
		delete(need, f.Name)
		if !f.IsEquivalent(e, true, true) {
			t.Errorf("Wrong needed file, got %v, expected %v", f, e)
		}
		return true
	})
	for n := range need {
		t.Errorf(`Missing needed file "%v"`, n)
	}
}

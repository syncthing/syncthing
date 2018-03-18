// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"testing"

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

// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"testing"
)

func TestDeviceKey(t *testing.T) {
	fld := []byte("folder6789012345678901234567890123456789012345678901234567890123")
	dev := []byte("device67890123456789012345678901")
	name := []byte("name")

	db := newLowlevelMemory(t)
	defer db.Close()

	key, err := db.keyer.GenerateDeviceFileKey(nil, fld, dev, name)
	if err != nil {
		t.Fatal(err)
	}

	fld2, ok := db.keyer.FolderFromDeviceFileKey(key)
	if !ok {
		t.Fatal("unexpectedly not found")
	}
	if !bytes.Equal(fld2, fld) {
		t.Errorf("wrong folder %q != %q", fld2, fld)
	}
	dev2, ok := db.keyer.DeviceFromDeviceFileKey(key)
	if !ok {
		t.Fatal("unexpectedly not found")
	}
	if !bytes.Equal(dev2, dev) {
		t.Errorf("wrong device %q != %q", dev2, dev)
	}
	name2 := db.keyer.NameFromDeviceFileKey(key)
	if !bytes.Equal(name2, name) {
		t.Errorf("wrong name %q != %q", name2, name)
	}
}

func TestGlobalKey(t *testing.T) {
	fld := []byte("folder6789012345678901234567890123456789012345678901234567890123")
	name := []byte("name")

	db := newLowlevelMemory(t)
	defer db.Close()

	key, err := db.keyer.GenerateGlobalVersionKey(nil, fld, name)
	if err != nil {
		t.Fatal(err)
	}

	name2 := db.keyer.NameFromGlobalVersionKey(key)
	if !bytes.Equal(name2, name) {
		t.Errorf("wrong name %q != %q", name2, name)
	}
}

func TestSequenceKey(t *testing.T) {
	fld := []byte("folder6789012345678901234567890123456789012345678901234567890123")

	db := newLowlevelMemory(t)
	defer db.Close()

	const seq = 1234567890
	key, err := db.keyer.GenerateSequenceKey(nil, fld, seq)
	if err != nil {
		t.Fatal(err)
	}
	outSeq := db.keyer.SequenceFromSequenceKey(key)
	if outSeq != seq {
		t.Errorf("sequence number mangled, %d != %d", outSeq, seq)
	}
}

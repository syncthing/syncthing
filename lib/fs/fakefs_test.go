// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"
)

func TestFakeFS(t *testing.T) {
	// Test some basic aspects of the fakefs

	fs := newFakeFilesystem("/foo/bar/baz")

	// MkdirAll
	err := fs.MkdirAll("dira/dirb", 0755)
	if err != nil {
		t.Fatal(err)
	}
	info, err := fs.Stat("dira/dirb")
	if err != nil {
		t.Fatal(err)
	}

	// Mkdir
	err = fs.Mkdir("dira/dirb/dirc", 0755)
	if err != nil {
		t.Fatal(err)
	}
	info, err = fs.Stat("dira/dirb/dirc")
	if err != nil {
		t.Fatal(err)
	}

	// Create
	fd, err := fs.Create("/dira/dirb/test")
	if err != nil {
		t.Fatal(err)
	}

	// Write
	_, err = fd.Write([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}

	// Stat on fd
	info, err = fd.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Name() != "test" {
		t.Error("wrong name:", info.Name())
	}
	if info.Size() != 5 {
		t.Error("wrong size:", info.Size())
	}

	// Stat on fs
	info, err = fs.Stat("dira/dirb/test")
	if err != nil {
		t.Fatal(err)
	}
	if info.Name() != "test" {
		t.Error("wrong name:", info.Name())
	}
	if info.Size() != 5 {
		t.Error("wrong size:", info.Size())
	}

	// Seek
	_, err = fd.Seek(1, os.SEEK_SET)
	if err != nil {
		t.Fatal(err)
	}

	// Read
	bs0, err := ioutil.ReadAll(fd)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs0) != 4 {
		t.Error("wrong number of bytes:", len(bs0))
	}

	// Read again, same data hopefully
	_, err = fd.Seek(0, os.SEEK_SET)
	if err != nil {
		t.Fatal(err)
	}
	bs1, err := ioutil.ReadAll(fd)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bs0, bs1[1:]) {
		t.Error("wrong data")
	}
}

func TestFakeFSRead(t *testing.T) {
	// Test some basic aspects of the fakefs

	fs := newFakeFilesystem("/foo/bar/baz")

	// Create
	fd, _ := fs.Create("test")
	fd.Truncate(3 * 1 << randomBlockShift)

	// Read
	fd.Seek(0, 0)
	bs0, err := ioutil.ReadAll(fd)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs0) != 3*1<<randomBlockShift {
		t.Error("wrong number of bytes:", len(bs0))
	}

	// Read again, starting at an odd offset
	fd.Seek(0, 0)
	buf0 := make([]byte, 12345)
	n, _ := fd.Read(buf0)
	if n != len(buf0) {
		t.Fatal("short read")
	}
	buf1, err := ioutil.ReadAll(fd)
	if err != nil {
		t.Fatal(err)
	}
	if len(buf1) != 3*1<<randomBlockShift-len(buf0) {
		t.Error("wrong number of bytes:", len(buf1))
	}

	bs1 := append(buf0, buf1...)
	if !bytes.Equal(bs0, bs1) {
		t.Error("data mismatch")
	}

	// Read large block with ReadAt
	bs2 := make([]byte, 3*1<<randomBlockShift)
	_, err = fd.ReadAt(bs2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bs0, bs2) {
		t.Error("data mismatch")
	}
}

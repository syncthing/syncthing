// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"os"
	"testing"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/sync"
)

func TestSourceFileOK(t *testing.T) {
	s := sharedPullerState{
		fs:       fs.NewFilesystem(fs.FilesystemTypeBasic, "testdata"),
		realName: "foo",
		mut:      sync.NewRWMutex(),
	}

	fd, err := s.sourceFile()
	if err != nil {
		t.Fatal(err)
	}
	if fd == nil {
		t.Fatal("Unexpected nil fd")
	}

	bs := make([]byte, 6)
	n, err := fd.Read(bs)
	if err != nil {
		t.Fatal(err)
	}

	if n != len(bs) {
		t.Fatalf("Wrong read length %d != %d", n, len(bs))
	}
	if string(bs) != "foobar" {
		t.Fatalf("Wrong contents %s != foobar", string(bs))
	}

	if err := s.failed(); err != nil {
		t.Fatal(err)
	}
}

func TestSourceFileBad(t *testing.T) {
	s := sharedPullerState{
		fs:       fs.NewFilesystem(fs.FilesystemTypeBasic, "testdata"),
		realName: "nonexistent",
		mut:      sync.NewRWMutex(),
	}

	fd, err := s.sourceFile()
	if err == nil {
		t.Fatal("Unexpected nil error")
	}
	if fd != nil {
		t.Fatal("Unexpected non-nil fd")
	}
	if err := s.failed(); err == nil {
		t.Fatal("Unexpected nil failed()")
	}
}

// Test creating temporary file inside read-only directory
func TestReadOnlyDir(t *testing.T) {
	// Create a read only directory, clean it up afterwards.
	os.Mkdir("testdata/read_only_dir", 0555)
	defer func() {
		os.Chmod("testdata/read_only_dir", 0755)
		os.RemoveAll("testdata/read_only_dir")
	}()

	s := sharedPullerState{
		fs:       fs.NewFilesystem(fs.FilesystemTypeBasic, "testdata"),
		tempName: "read_only_dir/.temp_name",
		mut:      sync.NewRWMutex(),
	}

	fd, err := s.tempFile()
	if err != nil {
		t.Fatal(err)
	}
	if fd == nil {
		t.Fatal("Unexpected nil fd")
	}

	s.fail("Test done", nil)
	s.finalClose()
}

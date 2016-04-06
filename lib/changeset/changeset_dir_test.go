// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package changeset

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestWriteDeleteDirectory(t *testing.T) {
	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	testWriteDeleteDirectory(t, testDir)
}

func TestWriteDeleteDirectoryDeep(t *testing.T) {
	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	deepDir := testDir
	deepDir.Name = "foo/bar/baz"

	cs := New(Options{
		RootPath:         "testdata",
		TempNamer:        defTempNamer,
		LocalRequester:   fakeRequester(testBlocks[:]),
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
	})

	// Applying the change set will fail as we are missing intermediate
	// directories.

	if err := cs.writeDir(deepDir); err == nil {
		t.Error("Unexpected nil error in writeDir")
	}
}

func TestWriteDeleteDirectoryReadOnly(t *testing.T) {
	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	if err := os.Mkdir("testdata/testdir", 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod("testdata/testdir", 0777)

	roDir := testDir
	roDir.Name = "testdir/bar"

	testWriteDeleteDirectory(t, roDir)
}

func testWriteDeleteDirectory(t *testing.T, d protocol.FileInfo) {
	cs := New(Options{
		RootPath:         "testdata",
		TempNamer:        defTempNamer,
		LocalRequester:   fakeRequester(testBlocks[:]),
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
	})

	// Create the directory

	if err := cs.writeDir(d); err != nil {
		t.Fatal(err)
	}

	info, err := os.Lstat(filepath.Join("testdata", d.Name))
	if err != nil {
		t.Fatal(err)
	}

	if !info.IsDir() {
		t.Error("Not a directory")
	}

	// Delete the directory

	if err := cs.deleteDir(d); err != nil {
		t.Fatal(err)
	}

	_, err = os.Lstat(filepath.Join("testdata", d.Name))
	if !os.IsNotExist(err) {
		t.Fatal("Directory still exists")
	}
}

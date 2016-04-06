// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package changeset

import (
	"os"
	"testing"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestWriteSymlinkToDir(t *testing.T) {
	// writeSymlink should be able to create a symlink to an existing
	// directory

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	if err := os.MkdirAll("testdata/target/of/symlink", 0777); err != nil {
		t.Fatal(err)
	}

	dirLink := testSymlink
	dirLink.Flags |= protocol.FlagDirectory

	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   fakeRequester(testBlocks[:]),
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
		TempNamer:        defTempNamer,
	})
	if err := cs.writeSymlink(dirLink); err != nil {
		t.Error(err)
	}

	target, targetType, err := fs.DefaultFilesystem.ReadSymlink("testdata/symlink")
	if err != nil {
		t.Error(err)
	}
	if target != "target/of/symlink" {
		t.Errorf("Incorrect target %q", target)
	}
	if targetType != fs.LinkTargetDirectory {
		t.Errorf("Incorrect target type %v", targetType)
	}
}

func TestWriteSymlinkToNonExistent(t *testing.T) {
	// writeSymlink should be able to create a symlink to an non existing
	// thing

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	cs := New(Options{
		RootPath:         "testdata",
		LocalRequester:   fakeRequester(testBlocks[:]),
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
		TempNamer:        defTempNamer,
	})
	if err := cs.writeSymlink(testSymlink); err != nil {
		t.Error(err)
	}

	target, targetType, err := fs.DefaultFilesystem.ReadSymlink("testdata/symlink")
	if err != nil {
		t.Error(err)
	}
	if target != "target/of/symlink" {
		t.Errorf("Incorrect target %q", target)
	}

	// Windows returns TargetFile, Unix returns TargetUnknown, for some
	// reason...?
	if targetType != fs.LinkTargetUnknown && targetType != fs.LinkTargetFile {
		t.Errorf("Incorrect target type %v", targetType)
	}
}

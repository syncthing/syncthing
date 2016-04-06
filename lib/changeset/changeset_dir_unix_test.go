// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// These tests require syscall.Umask

// +build !windows

package changeset

import (
	"os"
	"syscall"
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestUpdateDirectoryIgnorePerms(t *testing.T) {
	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")
	defer os.Chmod("testdata/dir", 0777)

	cs := New(Options{
		RootPath:         "testdata",
		TempNamer:        defTempNamer,
		LocalRequester:   fakeRequester(testBlocks[:]),
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
	})

	const umask = 0123
	oldMask := syscall.Umask(umask)
	defer syscall.Umask(oldMask)

	d := testDir
	d.Flags = 0345 | protocol.FlagNoPermBits

	if err := cs.writeDir(d); err != nil {
		t.Fatal(err)
	}

	if info, err := os.Lstat("testdata/dir"); err != nil {
		t.Fatal(err)
	} else if info.Mode()&0777 != 0777&^umask {
		t.Errorf("Directory should have 0%o, not 0%o permissions", 0777&^umask, info.Mode()&0777)
	}
}

func TestWriteDirectoryOverrideUmask(t *testing.T) {
	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")
	defer os.Chmod("testdata/dir", 0777)

	cs := New(Options{
		RootPath:         "testdata",
		TempNamer:        defTempNamer,
		LocalRequester:   fakeRequester(testBlocks[:]),
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
	})

	oldUmask := syscall.Umask(0776)
	defer syscall.Umask(oldUmask)

	if err := cs.writeDir(testDir); err != nil {
		t.Fatal(err)
	}

	if info, err := os.Lstat("testdata/dir"); err != nil {
		t.Fatal(err)
	} else if info.Mode()&0777 != os.FileMode(testDir.Flags&0777) {
		t.Errorf("Directory should have 0%o, not 0%o permissions", testDir.Flags&0777, info.Mode()&0777)
	}
}

func TestUpdateDirectoryExtraBits(t *testing.T) {
	// When changing permissions on a directory, the set-uid, set-gid and sticky bits should be preserved.

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	extraBits := os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	if err := os.Mkdir("testdata/dir", 0555|extraBits); err != nil {
		t.Fatal(err)
	}

	if info, err := os.Lstat("testdata/dir"); err != nil || info.Mode()&extraBits != extraBits {
		t.Skip("couldn't set extra bits")
	}

	defer os.Chmod("testdata/dir", 0777)

	cs := New(Options{
		RootPath:         "testdata",
		TempNamer:        defTempNamer,
		LocalRequester:   fakeRequester(testBlocks[:]),
		NetworkRequester: NewAsyncRequester(errorRequester{t}, 4),
	})

	if err := cs.writeDir(testDir); err != nil {
		t.Fatal(err)
	}

	info, err := os.Lstat("testdata/dir")
	if err != nil {
		t.Fatal(err)
	}

	// Mode should now be 0777 + the bits from above
	if info.Mode()&0777 != 0777 {
		t.Errorf("Incorrect mode 0%o, expecting 0777", info.Mode()&0777)
	}
	if info.Mode()&extraBits != extraBits {
		t.Errorf("Incorrect mode 0%o, expecting 0%o", info.Mode()&extraBits, extraBits)
	}
}

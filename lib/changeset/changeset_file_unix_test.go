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
	"github.com/syncthing/syncthing/lib/scanner"
)

func TestWriteFileIgnorePerms(t *testing.T) {
	// writeFile should not set permissions if the ignorePerms flag is set.

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	cs := New(Options{
		RootPath:       "testdata",
		TempNamer:      defTempNamer,
		LocalRequester: fakeRequester(testBlocks[:]),
	})

	const umask = 0124
	oldMask := syscall.Umask(umask)
	defer syscall.Umask(oldMask)

	f := testFile
	f.Flags = 0631 | protocol.FlagNoPermBits

	if err := cs.writeFile(f); err != nil {
		t.Error("Unexpected error from writeFile with local source:", err)
	}

	blocks, err := scanner.HashFile("testdata/test", protocol.BlockSize, 0, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !scanner.BlocksEqual(blocks, testFile.Blocks) {
		t.Error("Blocks differ after writeFile")
	}

	if info, err := os.Lstat("testdata/test"); err != nil {
		t.Fatal(err)
	} else if info.Mode()&0777 != 0666&^umask {
		t.Errorf("File should not have 0%o permissions", info.Mode())
	}
}

func TestWriteFilePermsOverrideUmask(t *testing.T) {
	// Permissions should take effect even with a restrictive umask

	os.RemoveAll("testdata")
	if err := os.Mkdir("testdata", 0777); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	cs := New(Options{
		RootPath:       "testdata",
		TempNamer:      defTempNamer,
		LocalRequester: fakeRequester(testBlocks[:]),
	})

	oldUmask := syscall.Umask(0776)
	defer syscall.Umask(oldUmask)

	if err := cs.writeFile(testFile); err != nil {
		t.Error("Unexpected error from writeFile with local source:", err)
	}

	blocks, err := scanner.HashFile("testdata/test", protocol.BlockSize, 0, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !scanner.BlocksEqual(blocks, testFile.Blocks) {
		t.Error("Blocks differ after writeFile")
	}

	if info, err := os.Lstat("testdata/test"); err != nil {
		t.Fatal(err)
	} else if info.Mode()&0777 != os.FileMode(testFile.Flags) {
		t.Errorf("File should not have 0%o permissions", info.Mode())
	}
}

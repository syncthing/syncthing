// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"testing"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/rand"
)

// Test creating temporary file inside read-only directory
func TestReadOnlyDir(t *testing.T) {
	ffs := fs.NewFilesystem(fs.FilesystemTypeFake, rand.String(32))
	ffs.Mkdir("testdir", 0o555)

	s := sharedPullerState{
		fs:       ffs,
		tempName: "testdir/.temp_name",
	}

	fd, err := s.tempFile()
	if err != nil {
		t.Fatal(err)
	}
	if fd == nil {
		t.Fatal("Unexpected nil fd")
	}

	s.fail(nil)
	s.finalClose()
}

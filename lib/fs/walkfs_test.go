// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"runtime"
	"testing"
)

func testWalkSkipSymlink(t *testing.T, fsType FilesystemType, uri string) {
	if runtime.GOOS == "windows" {
		t.Skip("Symlinks on windows")
	}

	fs := NewFilesystem(fsType, uri)

	if err := fs.MkdirAll("target/foo", 0755); err != nil {
		t.Fatal(err)
	}
	if err := fs.Mkdir("towalk", 0755); err != nil {
		t.Fatal(err)
	}
	if err := fs.CreateSymlink("target", "towalk/symlink"); err != nil {
		t.Fatal(err)
	}
	if err := fs.Walk("towalk", func(path string, info FileInfo, err error) error {
		if err != nil {
			t.Fatal(err)
		}
		if info.Name() != "symlink" && info.Name() != "towalk" {
			t.Fatal("Walk unexpected file", info.Name())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

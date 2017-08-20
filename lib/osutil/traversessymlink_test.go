// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build !windows

package osutil_test

import (
	"os"
	"testing"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/osutil"
)

func TestTraversesSymlink(t *testing.T) {
	os.RemoveAll("testdata")
	defer os.RemoveAll("testdata")

	fs := fs.NewFilesystem(fs.FilesystemTypeBasic, "testdata")
	fs.MkdirAll("a/b/c", 0755)
	fs.CreateSymlink("b", "a/l")

	// a/l -> b, so a/l/c should resolve by normal stat
	info, err := fs.Lstat("a/l/c")
	if err != nil {
		t.Fatal("unexpected error", err)
	}
	if !info.IsDir() {
		t.Fatal("error in setup, a/l/c should be a directory")
	}

	cases := []struct {
		name      string
		traverses bool
	}{
		// Exist
		{".", false},
		{"a", false},
		{"a/b", false},
		{"a/b/c", false},
		// Don't exist
		{"x", false},
		{"a/x", false},
		{"a/b/x", false},
		{"a/x/c", false},
		// Symlink or behind symlink
		{"a/l", true},
		{"a/l/c", true},
		// Non-existing behind a symlink
		{"a/l/x", true},
	}

	for _, tc := range cases {
		if res := osutil.TraversesSymlink(fs, tc.name); tc.traverses == (res == nil) {
			t.Errorf("TraversesSymlink(%q) = %v, should be %v", tc.name, res, tc.traverses)
		}
	}
}

var traversesSymlinkResult error

func BenchmarkTraversesSymlink(b *testing.B) {
	os.RemoveAll("testdata")
	defer os.RemoveAll("testdata")
	fs := fs.NewFilesystem(fs.FilesystemTypeBasic, "testdata")
	fs.MkdirAll("a/b/c", 0755)

	for i := 0; i < b.N; i++ {
		traversesSymlinkResult = osutil.TraversesSymlink(fs, "a/b/c")
	}

	b.ReportAllocs()
}

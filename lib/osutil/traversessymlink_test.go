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

	ffs := fs.NewFilesystem(fs.FilesystemTypeBasic, "testdata")
	ffs.MkdirAll("a/b/c", 0755)
	ffs.CreateSymlink("b", "a/l")

	// a/l -> b, so a/l/c should resolve by normal stat
	info, err := ffs.Lstat("a/l/c")
	if err != nil {
		t.Fatal("unexpected error", err)
	}
	if !info.IsDir() {
		t.Fatal("error in setup, a/l/c should be a directory")
	}

	cases := []struct {
		name       string
		symlink    bool
		isNotExist bool
		path       string
	}{
		// Exist
		{".", false, false, ""},
		{"a", false, false, ""},
		{"a/b", false, false, ""},
		{"a/b/c", false, false, ""},
		// Don't exist
		{"x", false, true, "x"},
		{"a/x", false, true, "a/x"},
		{"a/b/x", false, true, "a/b/x"},
		{"a/x/c", false, true, "a/x"},
		// Symlink or behind symlink
		{"a/l", true, false, "a/l"},
		{"a/l/c", true, false, "a/l"},
		// Non-existing behind a symlink
		{"a/l/x", true, false, "a/l"},
	}

	for _, tc := range cases {
		res := osutil.TraversesSymlink(ffs, tc.name)
		if (res != nil && fs.IsNotExist(res.Err)) != tc.isNotExist {
			t.Errorf("TraversesSymlink(%q) = \"%v\", %v, should report missing parent dir", tc.name, res, res.Path)
		}
		if (res != nil && res.Err == osutil.ErrTraversesSymlink) != tc.symlink {
			t.Errorf("TraversesSymlink(%q) = \"%v\", %v, should report traversed symlink", tc.name, res, res.Path)
		}
		if res != nil && res.Path != tc.path {
			t.Errorf("TraversesSymlink(%q) = \"%v\", %v, path should be %v", tc.name, res, res.Path, tc.path)
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

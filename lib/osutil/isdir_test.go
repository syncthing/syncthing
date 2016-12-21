// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package osutil_test

import (
	"os"
	"testing"

	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/symlinks"
)

func TestIsDir(t *testing.T) {
	if !symlinks.Supported {
		t.Skip("pointless test")
		return
	}

	os.RemoveAll("testdata")
	defer os.RemoveAll("testdata")
	os.MkdirAll("testdata/a/b/c", 0755)
	symlinks.Create("testdata/a/l", "b", symlinks.TargetDirectory)

	// a/l -> b, so a/l/c should resolve by normal stat
	info, err := osutil.Lstat("testdata/a/l/c")
	if err != nil {
		t.Fatal("unexpected error", err)
	}
	if !info.IsDir() {
		t.Fatal("error in setup, a/l/c should be a directory")
	}

	cases := []struct {
		name  string
		isDir bool
	}{
		// Exist
		{".", true},
		{"a", true},
		{"a/b", true},
		{"a/b/c", true},
		// Don't exist
		{"x", false},
		{"a/x", false},
		{"a/b/x", false},
		{"a/x/c", false},
		// Symlink or behind symlink
		{"a/l", false},
		{"a/l/c", false},
	}

	for _, tc := range cases {
		if res := osutil.IsDir("testdata", tc.name); res != tc.isDir {
			t.Errorf("IsDir(%q) = %v, should be %v", tc.name, res, tc.isDir)
		}
	}
}

var isDirResult bool

func BenchmarkIsDir(b *testing.B) {
	os.RemoveAll("testdata")
	defer os.RemoveAll("testdata")
	os.MkdirAll("testdata/a/b/c", 0755)

	for i := 0; i < b.N; i++ {
		isDirResult = osutil.IsDir("testdata", "a/b/c")
	}

	b.ReportAllocs()
}

// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package osutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/syncthing/syncthing/lib/osutil"
)

func TestCheckNameConflict(t *testing.T) {
	os.RemoveAll("testdata")
	defer os.RemoveAll("testdata")
	os.MkdirAll("testdata/Foo/BAR", 0755)

	cases := []struct {
		name         string
		conflictFree bool
	}{
		// Exists
		{"Foo", true},
		{"Foo/BAR", true},
		{"Foo/BAR/baz", true},
		// Doesn't exist
		{"bar", true},
		{"Foo/baz", true},
	}

	for _, tc := range cases {
		nativeName := filepath.FromSlash(tc.name)
		if res := osutil.CheckNameConflict("testdata", nativeName); res != tc.conflictFree {
			t.Errorf("CheckNameConflict(%q) = %v, should be %v", tc.name, res, tc.conflictFree)
		}
	}
}

func TestCheckNameConflictCasing(t *testing.T) {
	os.RemoveAll("testdata")
	defer os.RemoveAll("testdata")
	os.MkdirAll("testdata/Foo/BAR/baz", 0755)
	// check if the file system is case-sensitive
	if _, err := os.Lstat("testdata/foo"); err != nil {
		t.Skip("pointless test")
		return
	}

	cases := []struct {
		name         string
		conflictFree bool
	}{
		// Conflicts
		{"foo", false},
		{"foo/BAR", false},
		{"Foo/bar", false},
		{"Foo/BAR/BAZ", false},
	}

	for _, tc := range cases {
		nativeName := filepath.FromSlash(tc.name)
		if res := osutil.CheckNameConflict("testdata", nativeName); res != tc.conflictFree {
			t.Errorf("CheckNameConflict(%q) = %v, should be %v", tc.name, res, tc.conflictFree)
		}
	}
}

// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build !solaris,!darwin solaris,cgo darwin,cgo

package fswatcher

import (
	"path/filepath"
	"testing"
)

type paths struct {
	fullSubPath     string
	folderPath      string
	expectedSubPath string
}

func TestRelativeSubPath(t *testing.T) {
	pathSets := []paths{
		{"/home/user/Sync/blah", "/home/user/Sync/", "blah"},
		{"/home/user/Sync/blah", "/home/user/Sync", "blah"},
		{"/home/user/Sync/blah/", "/home/user/Sync/", "blah"},
		{"/home/user/Sync/blah/", "/home/user/Sync", "blah"},
		{"/home/user/Sync", "/home/user/Sync", "."},
		{"/home/user/Sync/", "/home/user/Sync", "."},
		{"/home/user/Sync", "/home/user/Sync/", "."},
		{"/home/user/Sync/", "/home/user/Sync/", "."},
	}
	for _, paths := range pathSets {
		paths.fullSubPath = filepath.Clean(paths.fullSubPath)
		paths.folderPath = filepath.Clean(paths.folderPath)
		paths.expectedSubPath = filepath.Clean(paths.expectedSubPath)
		result, _ := filepath.Rel(paths.folderPath, paths.fullSubPath)
		if result != paths.expectedSubPath {
			t.Errorf("Given: sub-path: '%s', folder path: '%s';\n got: '%s' expected '%s'",
				paths.fullSubPath, paths.folderPath,
				result, paths.expectedSubPath)
		}
	}
}

type subpathTest struct {
	folderPath string
	subPath    string
	isSubpath  bool
}

func TestIsSubpath(t *testing.T) {
	tests := []subpathTest{
		{"/home/user/Sync", "/home/user/Sync/blah", true},
		{"/home/user/Sync/", "/home/user/Sync/blah", true},
		{"/home/user/Sync/", "/home/user/Sync/", true},
		{"/home/user/Sync", "/home/user/Sync", true},
		{"/home/user/Sync", "/home/user/Sync/", true},
		{"/home/user/Sync/", "/home/user/Sync", true},
		{"/home/user/Sync/", "/another/path/Sync", false},
		{"/", "/", true},
		{"/", "//", true},
		{"/", "/some/path/blah", true},
	}
	for _, test := range tests {
		test.folderPath = filepath.Clean(test.folderPath)
		test.subPath = filepath.Clean(test.subPath)
		result := isSubpath(test.subPath, test.folderPath)
		if result != test.isSubpath {
			if test.isSubpath {
				t.Errorf("'%s' should be a subpath of '%s'\n",
					test.subPath, test.folderPath)
			} else {
				t.Errorf("'%s' should not be a subpath of '%s'\n",
					test.subPath, test.folderPath)
			}
		}
	}
}

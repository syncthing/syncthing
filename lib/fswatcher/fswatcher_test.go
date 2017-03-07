// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

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
	// TODO: same for windows
	pathSets := []paths{
		paths{"/home/user/Sync/blah", "/home/user/Sync/", "blah"},
		paths{"/home/user/Sync/blah", "/home/user/Sync", "blah"},
		paths{"/home/user/Sync/blah/", "/home/user/Sync/", "blah"},
		paths{"/home/user/Sync/blah/", "/home/user/Sync", "blah"},
		paths{"/home/user/Sync", "/home/user/Sync", "."},
		paths{"/home/user/Sync/", "/home/user/Sync", "."},
		paths{"/home/user/Sync", "/home/user/Sync/", "."},
		paths{"/home/user/Sync/", "/home/user/Sync/", "."},
	}
	for _, paths := range pathSets {
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
	// TODO: same for windows
	tests := []subpathTest{
		subpathTest{"/home/user/Sync", "/home/user/Sync/blah", true},
		subpathTest{"/home/user/Sync/", "/home/user/Sync/blah", true},
		subpathTest{"/home/user/Sync/", "/home/user/Sync/", true},
		subpathTest{"/home/user/Sync", "/home/user/Sync", true},
		subpathTest{"/home/user/Sync", "/home/user/Sync/", true},
		subpathTest{"/home/user/Sync/", "/home/user/Sync", true},
		subpathTest{"/home/user/Sync/", "/another/path/Sync", false},
		subpathTest{"/", "/", true},
		subpathTest{"/", "//", true},
		subpathTest{"/", "/some/path/blah", true},
	}
	for _, test := range tests {
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

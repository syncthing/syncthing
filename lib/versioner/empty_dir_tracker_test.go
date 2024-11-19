// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"path/filepath"
	"testing"

	"github.com/d4l3k/messagediff"
)

// TestEmptyDirs models the following .stversions structure:
//
//	.stversions/
//	├── keep1
//	│   └── file1
//	├── keep2
//	│   └── keep21
//	│       └── keep22
//	│           └── file1
//	├── remove1
//	└── remove2
//		└── remove21
//		    └── remove22
func TestEmptyDirs(t *testing.T) {
	paths := []struct {
		path   string
		isFile bool
	}{
		{".", false},
		{"keep1", false},
		{"keep1/file1", true},
		{"keep2", false},
		{"keep2/keep21", false},
		{"keep2/keep21/keep22", false},
		{"keep2/keep21/keep22/file1", true},
		{"remove1", false},
		{"remove2", false},
		{"remove2/remove21", false},
		{"remove2/remove21/remove22", false},
	}

	expected := []string{
		"remove2/remove21/remove22",
		"remove2/remove21",
		"remove2",
		"remove1",
	}

	// For compatibility with Windows
	for i, p := range paths {
		paths[i].path = filepath.FromSlash(p.path)
	}

	for i, p := range expected {
		expected[i] = filepath.FromSlash(p)
	}

	dirTracker := make(emptyDirTracker)
	for _, p := range paths {
		if p.isFile {
			dirTracker.addFile(p.path)
		} else {
			dirTracker.addDir(p.path)
		}
	}

	result := dirTracker.emptyDirs()
	if diff, equal := messagediff.PrettyDiff(expected, result); !equal {
		t.Errorf("Incorrect empty directories list; got %v, expected %v\n%v", result, expected, diff)
	}
}

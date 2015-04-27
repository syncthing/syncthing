// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package pruner

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPruner(t *testing.T) {
	patterns := []string{
		"some/deep/directory",
		"some/deep/other/directory",
		"some/files/[^/]+",
		"[^/]+",
	}

	tests := []struct {
		path       string
		isdir      bool
		shouldskip bool
	}{
		{"some/deep", true, false},
		{"some/deep/directory", true, false},
		{"some/deep/directory/anotherdir", true, false},
		{"some/deep/directory/file", false, false},
		{"some", true, false},

		{"some/file", false, true},
		{"some/deep/other/directory", true, false},
		{"some/deep/other/directory/anotherdir", true, false},
		{"some/deep/other/directory/file", false, false},
		{"some/deep/other/file", false, true},

		{"some/files/file1", false, false},
		{"some/files/file2", false, false},
		{"some/files/dir", true, true},
		{"some/files/dir/subdir", true, true},

		{"file", false, false},
		{"otherdir", true, true},
	}

	m := New(patterns)

	for _, test := range tests {
		if test.isdir {
			r := m.ShouldSkipDirectory(test.path)
			if r != test.shouldskip {
				t.Errorf("Mismatch: %t != %t for dir %q", r, test.shouldskip, test.path)
			}
			r = m.ShouldSkipDirectory(test.path + "/")
			if r != test.shouldskip {
				t.Errorf("Mismatch: %t != %t for dir %q", r, test.shouldskip, test.path)
			}
		} else {
			r := m.ShouldSkipFile(test.path)
			if r != test.shouldskip {
				t.Errorf("Mismatch: %t != %t for file %q", r, test.shouldskip, test.path)
			}
		}
	}

	// Add a trailing slash to all directory patterns
	for i := range patterns {
		if !strings.HasSuffix(patterns[i], "[^/]+") {
			patterns[i] += "/"
		}
	}

	// Convert test paths to Windows
	for i := range tests {
		tests[i].path = filepath.FromSlash(tests[i].path)
	}

	m = New(patterns)

	for _, test := range tests {
		if test.isdir {
			r := m.ShouldSkipDirectory(test.path)
			if r != test.shouldskip {
				t.Errorf("Mismatch: %t != %t for dir %q", r, test.shouldskip, test.path)
			}
			r = m.ShouldSkipDirectory(test.path + "/")
			if r != test.shouldskip {
				t.Errorf("Mismatch: %t != %t for dir %q", r, test.shouldskip, test.path)
			}
		} else {
			r := m.ShouldSkipFile(test.path)
			if r != test.shouldskip {
				t.Errorf("Mismatch: %t != %t for file %q", r, test.shouldskip, test.path)
			}
		}
	}
}

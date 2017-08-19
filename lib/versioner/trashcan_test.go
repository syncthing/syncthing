// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/fs"
)

func TestTrashcanCleanout(t *testing.T) {
	// Verify that files older than the cutoff are removed, that files newer
	// than the cutoff are *not* removed, and that empty directories are
	// removed (best effort).

	var testcases = []struct {
		file         string
		shouldRemove bool
	}{
		{"testdata/.stversions/file1", false},
		{"testdata/.stversions/file2", true},
		{"testdata/.stversions/keep1/file1", false},
		{"testdata/.stversions/keep1/file2", false},
		{"testdata/.stversions/keep2/file1", false},
		{"testdata/.stversions/keep2/file2", true},
		{"testdata/.stversions/remove/file1", true},
		{"testdata/.stversions/remove/file2", true},
	}

	os.RemoveAll("testdata")
	defer os.RemoveAll("testdata")

	oldTime := time.Now().Add(-8 * 24 * time.Hour)
	for _, tc := range testcases {
		os.MkdirAll(filepath.Dir(tc.file), 0777)
		if err := ioutil.WriteFile(tc.file, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
		if tc.shouldRemove {
			if err := os.Chtimes(tc.file, oldTime, oldTime); err != nil {
				t.Fatal(err)
			}
		}
	}

	versioner := NewTrashcan("default", fs.NewFilesystem(fs.FilesystemTypeBasic, "testdata"), map[string]string{"cleanoutDays": "7"}).(*Trashcan)
	if err := versioner.cleanoutArchive(); err != nil {
		t.Fatal(err)
	}

	for _, tc := range testcases {
		_, err := os.Lstat(tc.file)
		if tc.shouldRemove && !os.IsNotExist(err) {
			t.Error(tc.file, "should have been removed")
		} else if !tc.shouldRemove && err != nil {
			t.Error(tc.file, "should not have been removed")
		}
	}

	if _, err := os.Lstat("testdata/.stversions/remove"); !os.IsNotExist(err) {
		t.Error("empty directory should have been removed")
	}
}

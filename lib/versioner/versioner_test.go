// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/fs"
)

func TestVersionerCleanOut(t *testing.T) {
	simpleCfg := config.FolderConfiguration{
		FilesystemType: fs.FilesystemTypeBasic,
		Path:           "testdata",
		Versioning: config.VersioningConfiguration{
			Params: map[string]string{
				"cleanoutDays": "7",
			},
		},
	}

	trashCfg := config.FolderConfiguration{
		FilesystemType: fs.FilesystemTypeBasic,
		Path:           "testdata",
		Versioning: config.VersioningConfiguration{
			Params: map[string]string{
				"cleanoutDays": "7",
			},
		},
	}
	testCasesVersioner := []Versioner{
		newSimple(simpleCfg),
		newTrashcan(trashCfg).(*trashcan),
	}

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
		{"testdata/.stversions/keep3/keepsubdir/file1", false},
		{"testdata/.stversions/remove/file1", true},
		{"testdata/.stversions/remove/file2", true},
		{"testdata/.stversions/remove/removesubdir/file1", true},
	}

	for index, versioner := range testCasesVersioner {
		t.Run(fmt.Sprintf("versioner trashcan clean up %d in %d", index, len(testCasesVersioner)), func(t *testing.T) {
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

			if err := versioner.Clean(context.Background()); err != nil {
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

			if _, err := os.Lstat("testdata/.stversions/keep3"); os.IsNotExist(err) {
				t.Error("directory with non empty subdirs should not be removed")
			}

			if _, err := os.Lstat("testdata/.stversions/remove"); !os.IsNotExist(err) {
				t.Error("empty directory should have been removed")
			}
		})
	}
}

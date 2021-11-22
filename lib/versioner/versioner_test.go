// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/fs"
)

func TestVersionerCleanOut(t *testing.T) {
	cfg := config.FolderConfiguration{
		FilesystemType: fs.FilesystemTypeBasic,
		Path:           "testdata",
		Versioning: config.VersioningConfiguration{
			Params: map[string]string{
				"cleanoutDays": "7",
			},
		},
	}

	testCasesVersioner := map[string]Versioner{
		"simple":   newSimple(cfg),
		"trashcan": newTrashcan(cfg),
	}

	var testcases = map[string]bool{
		"testdata/.stversions/file1":                     false,
		"testdata/.stversions/file2":                     true,
		"testdata/.stversions/keep1/file1":               false,
		"testdata/.stversions/keep1/file2":               false,
		"testdata/.stversions/keep2/file1":               false,
		"testdata/.stversions/keep2/file2":               true,
		"testdata/.stversions/keep3/keepsubdir/file1":    false,
		"testdata/.stversions/remove/file1":              true,
		"testdata/.stversions/remove/file2":              true,
		"testdata/.stversions/remove/removesubdir/file1": true,
	}

	for versionerType, versioner := range testCasesVersioner {
		t.Run(fmt.Sprintf("%v versioner trashcan clean up", versionerType), func(t *testing.T) {
			os.RemoveAll("testdata")
			defer os.RemoveAll("testdata")

			oldTime := time.Now().Add(-8 * 24 * time.Hour)
			for file, shouldRemove := range testcases {
				os.MkdirAll(filepath.Dir(file), 0777)
				if err := os.WriteFile(file, []byte("data"), 0644); err != nil {
					t.Fatal(err)
				}
				if shouldRemove {
					if err := os.Chtimes(file, oldTime, oldTime); err != nil {
						t.Fatal(err)
					}
				}
			}

			if err := versioner.Clean(context.Background()); err != nil {
				t.Fatal(err)
			}

			for file, shouldRemove := range testcases {
				_, err := os.Lstat(file)
				if shouldRemove && !os.IsNotExist(err) {
					t.Error(file, "should have been removed")
				} else if !shouldRemove && err != nil {
					t.Error(file, "should not have been removed")
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

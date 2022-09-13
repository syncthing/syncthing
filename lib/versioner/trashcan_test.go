// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/fs"
)

func TestTrashcanArchiveRestoreSwitcharoo(t *testing.T) {
	// This tests that trashcan versioner restoration correctly archives existing file, because trashcan versioner
	// files are untagged, archiving existing file to replace with a restored version technically should collide in
	// in names.
	tmpDir1 := t.TempDir()

	tmpDir2 := t.TempDir()

	cfg := config.FolderConfiguration{
		FilesystemType: fs.FilesystemTypeBasic,
		Path:           tmpDir1,
		Versioning: config.VersioningConfiguration{
			FSType: fs.FilesystemTypeBasic,
			FSPath: tmpDir2,
		},
	}
	folderFs := cfg.Filesystem(nil)

	versionsFs := fs.NewFilesystem(fs.FilesystemTypeBasic, tmpDir2)

	writeFile(t, folderFs, "file", "A")

	versioner := newTrashcan(cfg)

	if err := versioner.Archive("file"); err != nil {
		t.Fatal(err)
	}

	if _, err := folderFs.Stat("file"); !fs.IsNotExist(err) {
		t.Fatal(err)
	}

	// Check versions
	versions, err := versioner.GetVersions()
	if err != nil {
		t.Fatal(err)
	}

	fileVersions := versions["file"]
	if len(fileVersions) != 1 {
		t.Fatalf("unexpected number of versions: %d != 1", len(fileVersions))
	}

	fileVersion := fileVersions[0]

	if !fileVersion.ModTime.Equal(fileVersion.VersionTime) {
		t.Error("time mismatch")
	}

	if content := readFile(t, versionsFs, "file"); content != "A" {
		t.Errorf("expected A got %s", content)
	}

	writeFile(t, folderFs, "file", "B")

	versionInfo, err := versionsFs.Stat("file")
	if err != nil {
		t.Fatal(err)
	}

	if !versionInfo.ModTime().Truncate(time.Second).Equal(fileVersion.ModTime) {
		t.Error("time mismatch")
	}

	if err := versioner.Restore("file", fileVersion.VersionTime); err != nil {
		t.Fatal(err)
	}

	if content := readFile(t, folderFs, "file"); content != "A" {
		t.Errorf("expected A got %s", content)
	}

	if content := readFile(t, versionsFs, "file"); content != "B" {
		t.Errorf("expected B got %s", content)
	}
}

func readFile(t *testing.T, filesystem fs.Filesystem, name string) string {
	t.Helper()
	fd, err := filesystem.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer fd.Close()
	buf, err := io.ReadAll(fd)
	if err != nil {
		t.Fatal(err)
	}
	return string(buf)
}

func writeFile(t *testing.T, filesystem fs.Filesystem, name, content string) {
	fd, err := filesystem.OpenFile(name, fs.OptReadWrite|fs.OptCreate, 0777)
	if err != nil {
		t.Fatal(err)
	}
	defer fd.Close()
	if err := fd.Truncate(int64(len(content))); err != nil {
		t.Fatal(err)
	}

	if n, err := fd.Write([]byte(content)); err != nil || n != len(content) {
		t.Fatal(n, len(content), err)
	}
}

func TestTrashcanCleanOut(t *testing.T) {

	testDir := t.TempDir()

	cfg := config.FolderConfiguration{
		FilesystemType: fs.FilesystemTypeBasic,
		Path:           testDir,
		Versioning: config.VersioningConfiguration{
			Params: map[string]string{
				"cleanoutDays": "7",
			},
		},
	}

	fs := cfg.Filesystem(nil)

	v := newTrashcan(cfg)

	var testcases = map[string]bool{
		".stversions/file1":                     false,
		".stversions/file2":                     true,
		".stversions/keep1/file1":               false,
		".stversions/keep1/file2":               false,
		".stversions/keep2/file1":               false,
		".stversions/keep2/file2":               true,
		".stversions/keep3/keepsubdir/file1":    false,
		".stversions/remove/file1":              true,
		".stversions/remove/file2":              true,
		".stversions/remove/removesubdir/file1": true,
	}

	t.Run(fmt.Sprintf("trashcan versioner trashcan clean up"), func(t *testing.T) {
		fs.RemoveAll("testdata")
		defer fs.RemoveAll("testdata")

		oldTime := time.Now().Add(-8 * 24 * time.Hour)
		for file, shouldRemove := range testcases {
			fs.MkdirAll(filepath.Dir(file), 0777)
			fs.Create(file)

			if shouldRemove {
				if err := fs.Chtimes(file, oldTime, oldTime); err != nil {
					t.Fatal(err)
				}
			}
		}

		if err := v.Clean(context.Background()); err != nil {
			t.Fatal(err)
		}

		for file, shouldRemove := range testcases {
			_, err := fs.Lstat(file)
			if shouldRemove && !os.IsNotExist(err) {
				t.Error(file, "should have been removed")
			} else if !shouldRemove && err != nil {
				t.Error(file, "should not have been removed")
			}
		}

		if _, err := fs.Lstat(".stversions/keep3"); os.IsNotExist(err) {
			t.Error("directory with non empty subdirs should not be removed")
		}

		if _, err := fs.Lstat(".stversions/remove"); !os.IsNotExist(err) {
			t.Error("empty directory should have been removed")
		}
	})
}

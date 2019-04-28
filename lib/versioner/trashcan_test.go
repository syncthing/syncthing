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
		{"testdata/.stversions/keep3/keepsubdir/file1", false},
		{"testdata/.stversions/remove/file1", true},
		{"testdata/.stversions/remove/file2", true},
		{"testdata/.stversions/remove/removesubdir/file1", true},
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

	if _, err := os.Lstat("testdata/.stversions/keep3"); os.IsNotExist(err) {
		t.Error("directory with non empty subdirs should not be removed")
	}

	if _, err := os.Lstat("testdata/.stversions/remove"); !os.IsNotExist(err) {
		t.Error("empty directory should have been removed")
	}
}

func TestTrashcanArchiveRestoreSwitcharoo(t *testing.T) {
	// This tests that trashcan versioner restoration correctly archives existing file, because trashcan versioner
	// files are untagged, archiving existing file to replace with a restored version technically should collide in
	// in names.
	tmpDir1, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}

	tmpDir2, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}

	folderFs := fs.NewFilesystem(fs.FilesystemTypeBasic, tmpDir1)
	versionsFs := fs.NewFilesystem(fs.FilesystemTypeBasic, tmpDir2)

	writeFile(t, folderFs, "file", "A")

	versioner := NewTrashcan("", folderFs, map[string]string{
		"fsType": "basic",
		"fsPath": tmpDir2,
	})

	if err := versioner.Archive("file"); err != nil {
		t.Fatal(err)
	}

	if _, err := folderFs.Stat("file"); !fs.IsNotExist(err) {
		t.Fatal(err)
	}

	versionInfo, err := versionsFs.Stat("file")
	if err != nil {
		t.Fatal(err)
	}

	if content := readFile(t, versionsFs, "file"); content != "A" {
		t.Errorf("expected A got %s", content)
	}

	writeFile(t, folderFs, "file", "B")

	if err := versioner.Restore("file", versionInfo.ModTime().Truncate(time.Second)); err != nil {
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
	fd, err := filesystem.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer fd.Close()
	buf, err := ioutil.ReadAll(fd)
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

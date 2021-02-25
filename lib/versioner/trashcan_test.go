// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"io/ioutil"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/fs"
)

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

	cfg := config.FolderConfiguration{
		FilesystemType: fs.FilesystemTypeBasic,
		Path:           tmpDir1,
		Versioning: config.VersioningConfiguration{
			FSType: fs.FilesystemTypeBasic,
			FSPath: tmpDir2,
		},
	}
	folderFs := cfg.Filesystem()

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

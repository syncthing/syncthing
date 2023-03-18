// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/fs"
)

func TestExternalNoCommand(t *testing.T) {
	file := "testdata/folder path/long filename.txt"
	prepForRemoval(t, file)
	defer os.RemoveAll("testdata")

	// The file should exist before the versioner run.

	if _, err := os.Lstat(file); err != nil {
		t.Fatal("File should exist")
	}

	// The versioner should fail due to missing command.

	e := external{
		filesystem: fs.NewFilesystem(fs.FilesystemTypeBasic, "."),
		command:    "nonexistent command",
	}
	if err := e.Archive(file); err == nil {
		t.Error("Command should have failed")
	}

	// The file should not have been removed.

	if _, err := os.Lstat(file); err != nil {
		t.Fatal("File should still exist")
	}
}

func TestExternal(t *testing.T) {
	cmd := "./_external_test/external.sh %FOLDER_PATH% %FILE_PATH%"
	if build.IsWindows {
		cmd = `.\\_external_test\\external.bat %FOLDER_PATH% %FILE_PATH%`
	}

	file := filepath.Join("testdata", "folder path", "dir (parens)", "/long filename (parens).txt")
	prepForRemoval(t, file)
	defer os.RemoveAll("testdata")

	// The file should exist before the versioner run.

	if _, err := os.Lstat(file); err != nil {
		t.Fatal("File should exist")
	}

	// The versioner should run successfully.

	e := external{
		filesystem: fs.NewFilesystem(fs.FilesystemTypeBasic, "."),
		command:    cmd,
	}
	if err := e.Archive(file); err != nil {
		t.Fatal(err)
	}

	// The file should no longer exist.

	if _, err := os.Lstat(file); !os.IsNotExist(err) {
		t.Error("File should no longer exist")
	}
}

func prepForRemoval(t *testing.T, file string) {
	if err := os.RemoveAll("testdata"); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

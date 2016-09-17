// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package versioner

import (
	"io/ioutil"
	"os"
	"runtime"
	"testing"
)

func TestExternalNoCommand(t *testing.T) {
	file := "testdata/folder path/long filename.txt"
	prepForRemoval(t, file)
	defer os.RemoveAll("testdata")

	if _, err := os.Lstat(file); err != nil {
		t.Fatal("File should exist")
	}

	e := External{
		command:    "nonexistant command",
		folderPath: "testdata/folder path",
	}
	if err := e.Archive(file); err == nil {
		t.Error("Command should have failed")
	}

	if _, err := os.Lstat(file); err != nil {
		t.Fatal("File should still exist")
	}
}

func TestExternal(t *testing.T) {
	cmd := "./_external_test/external.sh"
	if runtime.GOOS == "windows" {
		cmd = `.\_external_test\external.bat`
	}

	file := "testdata/folder path/long filename.txt"
	prepForRemoval(t, file)
	defer os.RemoveAll("testdata")

	if _, err := os.Lstat(file); err != nil {
		t.Fatal("File should exist")
	}

	e := External{
		command:    cmd,
		folderPath: "testdata/folder path",
	}
	if err := e.Archive(file); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(file); !os.IsNotExist(err) {
		t.Error("File should no longer exist")
	}
}

func prepForRemoval(t *testing.T, file string) {
	if err := os.RemoveAll("testdata"); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll("testdata/folder path", 0755); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(file, []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

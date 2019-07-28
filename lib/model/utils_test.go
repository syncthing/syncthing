// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"os"
	"runtime"
	"testing"

	"github.com/syncthing/syncthing/lib/fs"
)

func TestInWriteableDir(t *testing.T) {
	err := os.RemoveAll("testdata")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

	fs := fs.NewFilesystem(fs.FilesystemTypeBasic, ".")

	os.Mkdir("testdata", 0700)
	os.Mkdir("testdata/rw", 0700)
	os.Mkdir("testdata/ro", 0500)

	create := func(name string) error {
		fd, err := os.Create(name)
		if err != nil {
			return err
		}
		fd.Close()
		return nil
	}

	// These should succeed

	err = inWritableDir(create, fs, "testdata/file")
	if err != nil {
		t.Error("testdata/file:", err)
	}
	err = inWritableDir(create, fs, "testdata/rw/foo")
	if err != nil {
		t.Error("testdata/rw/foo:", err)
	}
	err = inWritableDir(os.Remove, fs, "testdata/rw/foo")
	if err != nil {
		t.Error("testdata/rw/foo:", err)
	}

	err = inWritableDir(create, fs, "testdata/ro/foo")
	if err != nil {
		t.Error("testdata/ro/foo:", err)
	}
	err = inWritableDir(os.Remove, fs, "testdata/ro/foo")
	if err != nil {
		t.Error("testdata/ro/foo:", err)
	}

	// These should not

	err = inWritableDir(create, fs, "testdata/nonexistent/foo")
	if err == nil {
		t.Error("testdata/nonexistent/foo returned nil error")
	}
	err = inWritableDir(create, fs, "testdata/file/foo")
	if err == nil {
		t.Error("testdata/file/foo returned nil error")
	}
}

func TestinWritableDirWindowsRemove(t *testing.T) {
	// os.Remove should remove read only things on windows

	if runtime.GOOS != "windows" {
		t.Skipf("Tests not required")
		return
	}

	err := os.RemoveAll("testdata")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chmod("testdata/windows/ro/readonlynew", 0700)
	defer os.RemoveAll("testdata")

	create := func(name string) error {
		fd, err := os.Create(name)
		if err != nil {
			return err
		}
		fd.Close()
		return nil
	}

	os.Mkdir("testdata", 0700)

	os.Mkdir("testdata/windows", 0500)
	os.Mkdir("testdata/windows/ro", 0500)
	create("testdata/windows/ro/readonly")
	os.Chmod("testdata/windows/ro/readonly", 0500)

	fs := fs.NewFilesystem(fs.FilesystemTypeBasic, ".")

	for _, path := range []string{"testdata/windows/ro/readonly", "testdata/windows/ro", "testdata/windows"} {
		err := inWritableDir(os.Remove, fs, path)
		if err != nil {
			t.Errorf("Unexpected error %s: %s", path, err)
		}
	}
}

func TestinWritableDirWindowsRemoveAll(t *testing.T) {
	// os.RemoveAll should remove read only things on windows

	if runtime.GOOS != "windows" {
		t.Skipf("Tests not required")
		return
	}

	err := os.RemoveAll("testdata")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chmod("testdata/windows/ro/readonlynew", 0700)
	defer os.RemoveAll("testdata")

	create := func(name string) error {
		fd, err := os.Create(name)
		if err != nil {
			return err
		}
		fd.Close()
		return nil
	}

	os.Mkdir("testdata", 0700)

	os.Mkdir("testdata/windows", 0500)
	os.Mkdir("testdata/windows/ro", 0500)
	create("testdata/windows/ro/readonly")
	os.Chmod("testdata/windows/ro/readonly", 0500)

	if err := os.RemoveAll("testdata/windows"); err != nil {
		t.Errorf("Unexpected error: %s", err)
	}
}

func TestinWritableDirWindowsRename(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skipf("Tests not required")
		return
	}

	err := os.RemoveAll("testdata")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chmod("testdata/windows/ro/readonlynew", 0700)
	defer os.RemoveAll("testdata")

	create := func(name string) error {
		fd, err := os.Create(name)
		if err != nil {
			return err
		}
		fd.Close()
		return nil
	}

	os.Mkdir("testdata", 0700)

	os.Mkdir("testdata/windows", 0500)
	os.Mkdir("testdata/windows/ro", 0500)
	create("testdata/windows/ro/readonly")
	os.Chmod("testdata/windows/ro/readonly", 0500)

	fs := fs.NewFilesystem(fs.FilesystemTypeBasic, ".")

	for _, path := range []string{"testdata/windows/ro/readonly", "testdata/windows/ro", "testdata/windows"} {
		err := os.Rename(path, path+"new")
		if err == nil {
			t.Skipf("seem like this test doesn't work here")
			return
		}
	}

	rename := func(path string) error {
		return fs.Rename(path, path+"new")
	}

	for _, path := range []string{"testdata/windows/ro/readonly", "testdata/windows/ro", "testdata/windows"} {
		err := inWritableDir(rename, fs, path)
		if err != nil {
			t.Errorf("Unexpected error %s: %s", path, err)
		}
		_, err = os.Stat(path + "new")
		if err != nil {
			t.Errorf("Unexpected error %s: %s", path, err)
		}
	}
}

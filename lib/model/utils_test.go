// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"runtime"
	"testing"

	"github.com/syncthing/syncthing/lib/fs"
)

func TestInWriteableDir(t *testing.T) {
	dir := t.TempDir()

	fs := fs.NewFilesystem(fs.FilesystemTypeBasic, dir)

	fs.Mkdir("testdata", 0700)
	fs.Mkdir("testdata/rw", 0700)
	fs.Mkdir("testdata/ro", 0500)

	create := func(name string) error {
		fd, err := fs.Create(name)
		if err != nil {
			return err
		}
		fd.Close()
		return nil
	}

	// These should succeed

	err := inWritableDir(create, fs, "testdata/file", false)
	if err != nil {
		t.Error("testdata/file:", err)
	}
	err = inWritableDir(create, fs, "testdata/rw/foo", false)
	if err != nil {
		t.Error("testdata/rw/foo:", err)
	}
	err = inWritableDir(fs.Remove, fs, "testdata/rw/foo", false)
	if err != nil {
		t.Error("testdata/rw/foo:", err)
	}

	err = inWritableDir(create, fs, "testdata/ro/foo", false)
	if err != nil {
		t.Error("testdata/ro/foo:", err)
	}
	err = inWritableDir(fs.Remove, fs, "testdata/ro/foo", false)
	if err != nil {
		t.Error("testdata/ro/foo:", err)
	}

	// These should not

	err = inWritableDir(create, fs, "testdata/nonexistent/foo", false)
	if err == nil {
		t.Error("testdata/nonexistent/foo returned nil error")
	}
	err = inWritableDir(create, fs, "testdata/file/foo", false)
	if err == nil {
		t.Error("testdata/file/foo returned nil error")
	}
}

func TestOSWindowsRemove(t *testing.T) {
	// os.Remove should remove read only things on windows

	if runtime.GOOS != "windows" {
		t.Skipf("Tests not required")
		return
	}

	dir := t.TempDir()

	fs := fs.NewFilesystem(fs.FilesystemTypeBasic, dir)
	defer fs.Chmod("testdata/windows/ro/readonlynew", 0700)

	create := func(name string) error {
		fd, err := fs.Create(name)
		if err != nil {
			return err
		}
		fd.Close()
		return nil
	}

	fs.Mkdir("testdata", 0700)

	fs.Mkdir("testdata/windows", 0500)
	fs.Mkdir("testdata/windows/ro", 0500)
	create("testdata/windows/ro/readonly")
	fs.Chmod("testdata/windows/ro/readonly", 0500)

	for _, path := range []string{"testdata/windows/ro/readonly", "testdata/windows/ro", "testdata/windows"} {
		err := inWritableDir(fs.Remove, fs, path, false)
		if err != nil {
			t.Errorf("Unexpected error %s: %s", path, err)
		}
	}
}

func TestOSWindowsRemoveAll(t *testing.T) {
	// os.RemoveAll should remove read only things on windows

	if runtime.GOOS != "windows" {
		t.Skipf("Tests not required")
		return
	}

	dir := t.TempDir()

	fs := fs.NewFilesystem(fs.FilesystemTypeBasic, dir)
	defer fs.Chmod("testdata/windows/ro/readonlynew", 0700)

	create := func(name string) error {
		fd, err := fs.Create(name)
		if err != nil {
			return err
		}
		fd.Close()
		return nil
	}

	fs.Mkdir("testdata", 0700)

	fs.Mkdir("testdata/windows", 0500)
	fs.Mkdir("testdata/windows/ro", 0500)
	create("testdata/windows/ro/readonly")
	fs.Chmod("testdata/windows/ro/readonly", 0500)

	if err := fs.RemoveAll("testdata/windows"); err != nil {
		t.Errorf("Unexpected error: %s", err)
	}
}

func TestInWritableDirWindowsRename(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skipf("Tests not required")
		return
	}

	dir := t.TempDir()

	fs := fs.NewFilesystem(fs.FilesystemTypeBasic, dir)
	defer fs.Chmod("testdata/windows/ro/readonlynew", 0700)

	create := func(name string) error {
		fd, err := fs.Create(name)
		if err != nil {
			return err
		}
		fd.Close()
		return nil
	}

	fs.Mkdir("testdata", 0700)

	fs.Mkdir("testdata/windows", 0500)
	fs.Mkdir("testdata/windows/ro", 0500)
	create("testdata/windows/ro/readonly")
	fs.Chmod("testdata/windows/ro/readonly", 0500)

	for _, path := range []string{"testdata/windows/ro/readonly", "testdata/windows/ro", "testdata/windows"} {
		err := fs.Rename(path, path+"new")
		if err == nil {
			t.Skipf("seem like this test doesn't work here")
			return
		}
	}

	rename := func(path string) error {
		return fs.Rename(path, path+"new")
	}

	for _, path := range []string{"testdata/windows/ro/readonly", "testdata/windows/ro", "testdata/windows"} {
		err := inWritableDir(rename, fs, path, false)
		if err != nil {
			t.Errorf("Unexpected error %s: %s", path, err)
		}
		_, err = fs.Stat(path + "new")
		if err != nil {
			t.Errorf("Unexpected error %s: %s", path, err)
		}
	}
}

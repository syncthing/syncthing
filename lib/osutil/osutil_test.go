// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package osutil_test

import (
	"os"
	"runtime"
	"testing"

	"github.com/syncthing/syncthing/lib/osutil"
)

func TestInWriteableDir(t *testing.T) {
	err := os.RemoveAll("testdata")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata")

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

	err = osutil.InWritableDir(create, "testdata/file")
	if err != nil {
		t.Error("testdata/file:", err)
	}
	err = osutil.InWritableDir(create, "testdata/rw/foo")
	if err != nil {
		t.Error("testdata/rw/foo:", err)
	}
	err = osutil.InWritableDir(os.Remove, "testdata/rw/foo")
	if err != nil {
		t.Error("testdata/rw/foo:", err)
	}

	err = osutil.InWritableDir(create, "testdata/ro/foo")
	if err != nil {
		t.Error("testdata/ro/foo:", err)
	}
	err = osutil.InWritableDir(os.Remove, "testdata/ro/foo")
	if err != nil {
		t.Error("testdata/ro/foo:", err)
	}

	// These should not

	err = osutil.InWritableDir(create, "testdata/nonexistent/foo")
	if err == nil {
		t.Error("testdata/nonexistent/foo returned nil error")
	}
	err = osutil.InWritableDir(create, "testdata/file/foo")
	if err == nil {
		t.Error("testdata/file/foo returned nil error")
	}
}

func TestInWritableDirWindowsRemove(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skipf("Tests not required")
		return
	}

	err := os.RemoveAll("testdata")
	if err != nil {
		t.Fatal(err)
	}
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

	for _, path := range []string{"testdata/windows/ro/readonly", "testdata/windows/ro", "testdata/windows"} {
		err := os.Remove(path)
		if err == nil {
			t.Errorf("Expected error %s", path)
		}
	}

	for _, path := range []string{"testdata/windows/ro/readonly", "testdata/windows/ro", "testdata/windows"} {
		err := osutil.InWritableDir(osutil.Remove, path)
		if err != nil {
			t.Errorf("Unexpected error %s: %s", path, err)
		}
	}
}

func TestInWritableDirWindowsRename(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skipf("Tests not required")
		return
	}

	err := os.RemoveAll("testdata")
	if err != nil {
		t.Fatal(err)
	}
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

	for _, path := range []string{"testdata/windows/ro/readonly", "testdata/windows/ro", "testdata/windows"} {
		err := os.Rename(path, path+"new")
		if err == nil {
			t.Skipf("seem like this test doesn't work here")
			return
		}
	}

	rename := func(path string) error {
		return osutil.Rename(path, path+"new")
	}

	for _, path := range []string{"testdata/windows/ro/readonly", "testdata/windows/ro", "testdata/windows"} {
		err := osutil.InWritableDir(rename, path)
		if err != nil {
			t.Errorf("Unexpected error %s: %s", path, err)
		}
		_, err = os.Stat(path + "new")
		if err != nil {
			t.Errorf("Unexpected error %s: %s", path, err)
		}
	}
}

func TestDiskUsage(t *testing.T) {
	free, err := osutil.DiskFreePercentage(".")
	if err != nil {
		if runtime.GOOS == "netbsd" ||
			runtime.GOOS == "openbsd" ||
			runtime.GOOS == "solaris" {
			t.Skip()
		}
		t.Errorf("Unexpected error: %s", err)
	}
	if free < 1 {
		t.Error("Disk is full?", free)
	}
}

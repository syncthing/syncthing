// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package osutil_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestCaseSensitiveStat(t *testing.T) {
	switch runtime.GOOS {
	case "windows", "darwin":
		break // We can test!
	default:
		t.Skip("Cannot test on this platform")
		return
	}

	dir, err := ioutil.TempDir("", "TestCaseSensitiveStat")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	if err := ioutil.WriteFile(filepath.Join(dir, "File"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(filepath.Join(dir, "File")); err != nil {
		// Standard Lstat should report the file exists
		t.Fatal("Unexpected error:", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "fILE")); err != nil {
		// ... also with the incorrect case spelling
		t.Fatal("Unexpected error:", err)
	}

	// Create the case sensitive stat:er. We stress it a little by giving it a
	// base path with an intentionally incorrect casing.

	css := osutil.NewCachedCaseSensitiveStat(strings.ToUpper(dir))

	if _, err := css.Lstat(filepath.Join(dir, "File")); err != nil {
		// Our Lstat should report the file exists
		t.Fatal("Unexpected error:", err)
	}
	if _, err := css.Lstat(filepath.Join(dir, "fILE")); err == nil || !os.IsNotExist(err) {
		// ... but with the incorrect case we should get ErrNotExist
		t.Fatal("Unexpected non-IsNotExist error:", err)
	}

	// Now do the same tests for a file in a case-sensitive directory.

	if err := os.Mkdir(filepath.Join(dir, "Dir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(dir, "Dir/File"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(filepath.Join(dir, "Dir/File")); err != nil {
		// Standard Lstat should report the file exists
		t.Fatal("Unexpected error:", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "dIR/File")); err != nil {
		// ... also with the incorrect case spelling
		t.Fatal("Unexpected error:", err)
	}

	// Recreate the case sensitive stat:er. We stress it a little by giving it a
	// base path with an intentionally incorrect casing.

	css = osutil.NewCachedCaseSensitiveStat(strings.ToLower(dir))

	if _, err := css.Lstat(filepath.Join(dir, "Dir/File")); err != nil {
		// Our Lstat should report the file exists
		t.Fatal("Unexpected error:", err)
	}
	if _, err := css.Lstat(filepath.Join(dir, "dIR/File")); err == nil || !os.IsNotExist(err) {
		// ... but with the incorrect case we should get ErrNotExist
		t.Fatal("Unexpected non-IsNotExist error:", err)
	}
}

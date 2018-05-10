// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/osutil"
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

	err = osutil.InWritableDir(create, fs, "testdata/file")
	if err != nil {
		t.Error("testdata/file:", err)
	}
	err = osutil.InWritableDir(create, fs, "testdata/rw/foo")
	if err != nil {
		t.Error("testdata/rw/foo:", err)
	}
	err = osutil.InWritableDir(os.Remove, fs, "testdata/rw/foo")
	if err != nil {
		t.Error("testdata/rw/foo:", err)
	}

	err = osutil.InWritableDir(create, fs, "testdata/ro/foo")
	if err != nil {
		t.Error("testdata/ro/foo:", err)
	}
	err = osutil.InWritableDir(os.Remove, fs, "testdata/ro/foo")
	if err != nil {
		t.Error("testdata/ro/foo:", err)
	}

	// These should not

	err = osutil.InWritableDir(create, fs, "testdata/nonexistent/foo")
	if err == nil {
		t.Error("testdata/nonexistent/foo returned nil error")
	}
	err = osutil.InWritableDir(create, fs, "testdata/file/foo")
	if err == nil {
		t.Error("testdata/file/foo returned nil error")
	}
}

func TestInWritableDirWindowsRemove(t *testing.T) {
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
		err := osutil.InWritableDir(os.Remove, fs, path)
		if err != nil {
			t.Errorf("Unexpected error %s: %s", path, err)
		}
	}
}

func TestInWritableDirWindowsRemoveAll(t *testing.T) {
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

func TestInWritableDirWindowsRename(t *testing.T) {
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
		return osutil.Rename(fs, path, path+"new")
	}

	for _, path := range []string{"testdata/windows/ro/readonly", "testdata/windows/ro", "testdata/windows"} {
		err := osutil.InWritableDir(rename, fs, path)
		if err != nil {
			t.Errorf("Unexpected error %s: %s", path, err)
		}
		_, err = os.Stat(path + "new")
		if err != nil {
			t.Errorf("Unexpected error %s: %s", path, err)
		}
	}
}

func TestIsDeleted(t *testing.T) {
	type tc struct {
		path  string
		isDel bool
	}
	cases := []tc{
		{"del", true},
		{"del.file", false},
		{"del/del", true},
		{"file", false},
		{"linkToFile", false},
		{"linkToDel", false},
		{"linkToDir", false},
		{"linkToDir/file", true},
		{"file/behindFile", true},
		{"dir", false},
		{"dir.file", false},
		{"dir/file", false},
		{"dir/del", true},
		{"dir/del/del", true},
		{"del/del/del", true},
	}

	testFs := fs.NewFilesystem(fs.FilesystemTypeBasic, "testdata")

	for _, f := range []string{"dir", "inacc"} {
		testFs.MkdirAll(f, 0777)
	}
	for _, f := range []string{"file", "del.file", "dir.file", "dir/file", "inacc/file"} {
		fd, err := testFs.Create(f)
		if err != nil {
			t.Fatal(err)
		}
		fd.Close()
	}
	if runtime.GOOS != "windows" {
		// Windows doesn't support 0000 dir permissions
		if err := testFs.Chmod("inacc", 0000); err != nil {
			t.Fatal(err)
		}
		if _, err := testFs.Lstat("inacc/file"); !fs.IsPermission(err) {
			t.Fatalf("not a permission error: %v", err)
		}
		cases = append(cases, tc{"inacc", false}, tc{"inacc/file", false})
	}
	for _, n := range []string{"Dir", "File", "Del"} {
		if err := osutil.DebugSymlinkForTestsOnly(filepath.Join(testFs.URI(), strings.ToLower(n)), filepath.Join(testFs.URI(), "linkTo"+n)); err != nil {
			if runtime.GOOS == "windows" {
				t.Skip("Symlinks aren't working")
			}
			t.Fatal(err)
		}
	}

	for _, c := range cases {
		if osutil.IsDeleted(testFs, c.path) != c.isDel {
			t.Errorf("IsDeleted(%v) != %v", c.path, c.isDel)
		}
	}

	testFs.Chmod("inacc", 0777)
	os.RemoveAll("testdata")
}

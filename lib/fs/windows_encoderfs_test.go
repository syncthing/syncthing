// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/rand"
)

func windowsSetup(t *testing.T) (*WindowsEncoderFilesystem, string) {
	t.Helper()
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}

	return newWindowsEncoderFilesystem(newBasicFilesystem(dir)), dir
}

func TestEncoderChmodFile(t *testing.T) {
	fs, dir := windowsSetup(t)
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "file")

	defer os.Chmod(path, 0666)

	fd, err := os.Create(path)
	if err != nil {
		t.Error(err)
	}
	fd.Close()

	if err := os.Chmod(path, 0666); err != nil {
		t.Error(err)
	}

	if stat, err := os.Stat(path); err != nil || stat.Mode()&os.ModePerm != 0666 {
		t.Errorf("wrong perm: %t %#o", err == nil, stat.Mode()&os.ModePerm)
	}

	if err := fs.Chmod("file", 0444); err != nil {
		t.Error(err)
	}

	if stat, err := os.Stat(path); err != nil || stat.Mode()&os.ModePerm != 0444 {
		t.Errorf("wrong perm: %t %#o", err == nil, stat.Mode()&os.ModePerm)
	}
}

func TestEncoderChownFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Not supported on Windows")
		return
	}
	if os.Getuid() != 0 {
		// We are not root. No expectation of being able to chown. Our tests
		// typically don't run with CAP_FOWNER.
		t.Skip("Test not possible")
		return
	}

	fs, dir := windowsSetup(t)
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "file")

	defer os.Chmod(path, 0666)

	fd, err := os.Create(path)
	if err != nil {
		t.Error("Unexpected error:", err)
	}
	fd.Close()

	_, err = fs.Lstat("file")
	if err != nil {
		t.Error("Unexpected error:", err)
	}

	newUID := 1000 + rand.Intn(30000)
	newGID := 1000 + rand.Intn(30000)

	if err := fs.Lchown("file", newUID, newGID); err != nil {
		t.Error("Unexpected error:", err)
	}

	info, err := fs.Lstat("file")
	if err != nil {
		t.Error("Unexpected error:", err)
	}
	if info.Owner() != newUID {
		t.Errorf("Incorrect owner, expected %d but got %d", newUID, info.Owner())
	}
	if info.Group() != newGID {
		t.Errorf("Incorrect group, expected %d but got %d", newGID, info.Group())
	}
}

func TestEncoderChmodDir(t *testing.T) {
	fs, dir := windowsSetup(t)
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "dir")

	mode := os.FileMode(0755)
	if runtime.GOOS == "windows" {
		mode = os.FileMode(0777)
	}

	defer os.Chmod(path, mode)

	if err := os.Mkdir(path, mode); err != nil {
		t.Error(err)
	}
	// On UNIX, Mkdir will subtract the umask, so force desired mode explicitly
	if err := os.Chmod(path, mode); err != nil {
		t.Error(err)
	}

	if stat, err := os.Stat(path); err != nil || stat.Mode()&os.ModePerm != mode {
		t.Errorf("wrong perm: %t %#o", err == nil, stat.Mode()&os.ModePerm)
	}

	if err := fs.Chmod("dir", 0555); err != nil {
		t.Error(err)
	}

	if stat, err := os.Stat(path); err != nil || stat.Mode()&os.ModePerm != 0555 {
		t.Errorf("wrong perm: %t %#o", err == nil, stat.Mode()&os.ModePerm)
	}
}

func TestEncoderChtimes(t *testing.T) {
	fs, dir := windowsSetup(t)
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "file")
	fd, err := os.Create(path)
	if err != nil {
		t.Error(err)
	}
	fd.Close()

	mtime := time.Now().Add(-time.Hour)

	fs.Chtimes("file", mtime, mtime)

	stat, err := os.Stat(path)
	if err != nil {
		t.Error(err)
	}

	diff := stat.ModTime().Sub(mtime)
	if diff > 3*time.Second || diff < -3*time.Second {
		t.Errorf("%s != %s", stat.Mode(), mtime)
	}
}

func TestEncoderCreate(t *testing.T) {
	fs, dir := windowsSetup(t)
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "file")

	if _, err := os.Stat(path); err == nil {
		t.Errorf("exists?")
	}

	fd, err := fs.Create("file")
	if err != nil {
		t.Error(err)
	}
	fd.Close()

	if _, err := os.Stat(path); err != nil {
		t.Error(err)
	}
}

func TestEncoderCreateSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows not supported")
	}

	fs, dir := windowsSetup(t)
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "file")

	if err := fs.CreateSymlink("blah", "file"); err != nil {
		t.Error(err)
	}

	if target, err := os.Readlink(path); err != nil || target != "blah" {
		t.Error("target", target, "err", err)
	}

	if err := os.Remove(path); err != nil {
		t.Error(err)
	}

	if err := fs.CreateSymlink(filepath.Join("..", "blah"), "file"); err != nil {
		t.Error(err)
	}

	if target, err := os.Readlink(path); err != nil || target != filepath.Join("..", "blah") {
		t.Error("target", target, "err", err)
	}
}

func TestEncoderDirNames(t *testing.T) {
	fs, dir := windowsSetup(t)
	defer os.RemoveAll(dir)

	// Case differences
	testCases := []string{
		"a",
		"bC",
	}
	sort.Strings(testCases)

	for _, sub := range testCases {
		if err := os.Mkdir(filepath.Join(dir, sub), 0777); err != nil {
			t.Error(err)
		}
	}

	if dirs, err := fs.DirNames("."); err != nil || len(dirs) != len(testCases) {
		t.Errorf("%s %s %s", err, dirs, testCases)
	} else {
		sort.Strings(dirs)
		for i := range dirs {
			if dirs[i] != testCases[i] {
				t.Errorf("%s != %s", dirs[i], testCases[i])
			}
		}
	}
}

func TestEncoderNames(t *testing.T) {
	// Tests that all names are without the root directory.
	fs, dir := windowsSetup(t)
	defer os.RemoveAll(dir)

	expected := "file"
	fd, err := fs.Create(expected)
	if err != nil {
		t.Error(err)
	}
	defer fd.Close()

	if fd.Name() != expected {
		t.Errorf("incorrect %s != %s", fd.Name(), expected)
	}
	if stat, err := fd.Stat(); err != nil || stat.Name() != expected {
		t.Errorf("incorrect %s != %s (%v)", stat.Name(), expected, err)
	}

	if err := fs.Mkdir("dir", 0777); err != nil {
		t.Error(err)
	}

	expected = filepath.Join("dir", "file")
	fd, err = fs.Create(expected)
	if err != nil {
		t.Error(err)
	}
	defer fd.Close()

	if fd.Name() != expected {
		t.Errorf("incorrect %s != %s", fd.Name(), expected)
	}

	// os.fd.Stat() returns just base, so do we.
	if stat, err := fd.Stat(); err != nil || stat.Name() != filepath.Base(expected) {
		t.Errorf("incorrect %s != %s (%v)", stat.Name(), filepath.Base(expected), err)
	}
}

func TestEncoderGlob(t *testing.T) {
	// Tests that all names are without the root directory.
	fs, dir := windowsSetup(t)
	defer os.RemoveAll(dir)

	for _, dirToCreate := range []string{
		filepath.Join("a", "test", "b"),
		filepath.Join("a", "best", "b"),
		filepath.Join("a", "best", "c"),
	} {
		if err := fs.MkdirAll(dirToCreate, 0777); err != nil {
			t.Error(err)
		}
	}

	testCases := []struct {
		pattern string
		matches []string
	}{
		{
			filepath.Join("a", "?est", "?"),
			[]string{
				filepath.Join("a", "test", "b"),
				filepath.Join("a", "best", "b"),
				filepath.Join("a", "best", "c"),
			},
		},
		{
			filepath.Join("a", "?est", "b"),
			[]string{
				filepath.Join("a", "test", "b"),
				filepath.Join("a", "best", "b"),
			},
		},
		{
			filepath.Join("a", "best", "?"),
			[]string{
				filepath.Join("a", "best", "b"),
				filepath.Join("a", "best", "c"),
			},
		},
	}

	for _, testCase := range testCases {
		results, err := fs.Glob(testCase.pattern)
		sort.Strings(results)
		sort.Strings(testCase.matches)
		if err != nil {
			t.Error(err)
		}
		if len(results) != len(testCase.matches) {
			t.Errorf("result count mismatch")
		}
		for i := range testCase.matches {
			if results[i] != testCase.matches[i] {
				t.Errorf("%s != %s", results[i], testCase.matches[i])
			}
		}
	}
}

func TestEncoderUsage(t *testing.T) {
	fs, dir := windowsSetup(t)
	defer os.RemoveAll(dir)
	usage, err := fs.Usage(".")
	if err != nil {
		if runtime.GOOS == "netbsd" || runtime.GOOS == "openbsd" || runtime.GOOS == "solaris" {
			t.Skip()
		}
		t.Errorf("Unexpected error: %s", err)
	}
	if usage.Free < 1 {
		t.Error("Disk is full?", usage.Free)
	}
}

func TestEncoderCreateReservedFiles(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Only supported on Windows")
		return
	}
	fs, dir := windowsSetup(t)
	// Adding ntNamespacePrefix reduces time from 20.690s to 0.02s:
	prefixedDir := dir
	if runtime.GOOS == "windows" {
		prefixedDir = ntNamespacePrefix + prefixedDir
	}
	defer os.RemoveAll(prefixedDir)
	for _, name := range windowsDisallowedNames {
		path := filepath.Join(dir, name)
		prefixedPath := path
		if runtime.GOOS == "windows" {
			prefixedPath = ntNamespacePrefix + prefixedPath
		}
		if _, err := os.Stat(prefixedPath); err == nil {
			t.Errorf("exists?")
		}

		fd, err := fs.Create(name)
		if err != nil {
			t.Error(err)
			return
		}
		fd.Close()

		if _, err := os.Stat(prefixedPath); err != nil {
			t.Error(err)
		}
	}
}

func TestEncoderCreateInvalidNames(t *testing.T) {
	fs, dir := windowsSetup(t)
	defer os.RemoveAll(dir)
	for _, r := range fs.reservedChars {
		name := string(r)
		path := filepath.Join(dir, name)

		if _, err := os.Stat(path); err == nil {
			t.Errorf("exists?")
		}

		fd, err := fs.Create(name)
		if err != nil {
			t.Error(err)
		}
		fd.Close()

		if _, err := fs.Stat(name); err != nil {
			t.Error(err)
		}

		// fails with: CreateFile : The system cannot find the file specified.
		// if _, err := os.Stat(expected); err != nil {
		//	t.Error(err)
		// }
		// but including the full path succeeds:
		if _, err := os.Stat(fs.encodedPath(path)); err != nil {
			t.Error(err)
		}
	}
}

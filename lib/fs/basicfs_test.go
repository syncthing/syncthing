// Copyright (C) 2017 The Syncthing Authors.
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
)

/*
	Chmod(name string, mode FileMode) error
	Chtimes(name string, atime time.Time, mtime time.Time) error
	Create(name string) (File, error)
	CreateSymlink(name, target string) error
	DirNames(name string) ([]string, error)
	Lstat(name string) (FileInfo, error)
	Mkdir(name string, perm FileMode) error
	MkdirAll(name string, perm FileMode) error
	Open(name string) (File, error)
	OpenFile(name string, flags int, mode FileMode) (File, error)
	ReadSymlink(name string) (string, error)
	Remove(name string) error
	RemoveAll(name string) error
	Rename(oldname, newname string) error
	Stat(name string) (FileInfo, error)
	SymlinksSupported() bool
	Walk(root string, walkFn WalkFunc) error
	Show(name string) error
	Hide(name string) error
	Glob(pattern string) ([]string, error)
	Roots() ([]string, error)
	Usage(name string) (Usage, error)
	Type() FilesystemType
	URI() string
*/

func setup(t *testing.T) (Filesystem, string) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	return NewBasicFilesystem(dir), dir
}

func TestChmodFile(t *testing.T) {
	fs, dir := setup(t)
	path := filepath.Join(dir, "file")
	defer os.RemoveAll(dir)

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

func TestChmodDir(t *testing.T) {
	fs, dir := setup(t)
	path := filepath.Join(dir, "dir")
	defer os.RemoveAll(dir)

	defer os.Chmod(path, 0666)

	if err := os.Mkdir(path, 0777); err != nil {
		t.Error(err)
	}

	if stat, err := os.Stat(path); err != nil || stat.Mode()&os.ModePerm != 0777 {
		t.Errorf("wrong perm: %t %#o", err == nil, stat.Mode()&os.ModePerm)
	}

	if err := fs.Chmod("dir", 0555); err != nil {
		t.Error(err)
	}

	if stat, err := os.Stat(path); err != nil || stat.Mode()&os.ModePerm != 0555 {
		t.Errorf("wrong perm: %t %#o", err == nil, stat.Mode()&os.ModePerm)
	}
}

func TestChtimes(t *testing.T) {
	fs, dir := setup(t)
	path := filepath.Join(dir, "file")
	defer os.RemoveAll(dir)
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

	if !stat.ModTime().Equal(mtime) {
		t.Errorf("%s != %s", stat.Mode(), mtime)
	}
}

func TestCreate(t *testing.T) {
	fs, dir := setup(t)
	path := filepath.Join(dir, "file")
	defer os.RemoveAll(dir)

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

func TestCreateSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows not supported")
	}

	fs, dir := setup(t)
	path := filepath.Join(dir, "file")
	defer os.RemoveAll(dir)

	if err := fs.CreateSymlink("file", "blah"); err != nil {
		t.Error(err)
	}

	if target, err := os.Readlink(path); err != nil || target != "blah" {
		t.Error("target", target, "err", err)
	}

	if err := os.Remove(path); err != nil {
		t.Error(err)
	}

	if err := fs.CreateSymlink("file", filepath.Join("..", "blah")); err != nil {
		t.Error(err)
	}

	if target, err := os.Readlink(path); err != nil || target != filepath.Join("..", "blah") {
		t.Error("target", target, "err", err)
	}
}

func TestDirNames(t *testing.T) {
	fs, dir := setup(t)
	defer os.RemoveAll(dir)

	// Case differences
	testCases := []string{
		"a",
		"bC",
	}

	for _, sub := range testCases {
		if err := os.Mkdir(filepath.Join(dir, sub), 0777); err != nil {
			t.Error(err)
		}
	}

	if dirs, err := fs.DirNames("."); err != nil || len(dirs) != len(testCases) {
		t.Errorf("%s %s %s", err, dirs, testCases)
	} else {
		for i := range dirs {
			if dirs[i] != testCases[i] {
				t.Errorf("%s != %s", dirs[i], testCases[i])
			}
		}
	}
}

func TestNames(t *testing.T) {
	// Tests that all names are without the root directory.
	fs, dir := setup(t)
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

func TestGlob(t *testing.T) {
	// Tests that all names are without the root directory.
	fs, dir := setup(t)
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

func TestUsage(t *testing.T) {
	fs, dir := setup(t)
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

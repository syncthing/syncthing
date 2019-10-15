// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"sort"
	"testing"
)

func TestFakeFS(t *testing.T) {
	// Test some basic aspects of the fakefs

	fs := newFakeFilesystem("/foo/bar/baz")

	// MkdirAll
	err := fs.MkdirAll("dira/dirb", 0755)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fs.Stat("dira/dirb")
	if err != nil {
		t.Fatal(err)
	}

	// Mkdir
	err = fs.Mkdir("dira/dirb/dirc", 0755)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fs.Stat("dira/dirb/dirc")
	if err != nil {
		t.Fatal(err)
	}

	// Create
	fd, err := fs.Create("/dira/dirb/test")
	if err != nil {
		t.Fatal(err)
	}

	// Write
	_, err = fd.Write([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}

	// Stat on fd
	info, err := fd.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Name() != "test" {
		t.Error("wrong name:", info.Name())
	}
	if info.Size() != 5 {
		t.Error("wrong size:", info.Size())
	}

	// Stat on fs
	info, err = fs.Stat("dira/dirb/test")
	if err != nil {
		t.Fatal(err)
	}
	if info.Name() != "test" {
		t.Error("wrong name:", info.Name())
	}
	if info.Size() != 5 {
		t.Error("wrong size:", info.Size())
	}

	// Seek
	_, err = fd.Seek(1, io.SeekStart)
	if err != nil {
		t.Fatal(err)
	}

	// Read
	bs0, err := ioutil.ReadAll(fd)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs0) != 4 {
		t.Error("wrong number of bytes:", len(bs0))
	}

	// Read again, same data hopefully
	_, err = fd.Seek(0, io.SeekStart)
	if err != nil {
		t.Fatal(err)
	}
	bs1, err := ioutil.ReadAll(fd)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bs0, bs1[1:]) {
		t.Error("wrong data")
	}

	// Create symlink
	if err := fs.CreateSymlink("foo", "dira/dirb/symlink"); err != nil {
		t.Fatal(err)
	}
	if str, err := fs.ReadSymlink("dira/dirb/symlink"); err != nil {
		t.Fatal(err)
	} else if str != "foo" {
		t.Error("Wrong symlink destination", str)
	}

	// Chown
	if err := fs.Lchown("dira", 1234, 5678); err != nil {
		t.Fatal(err)
	}
	if info, err := fs.Lstat("dira"); err != nil {
		t.Fatal(err)
	} else if info.Owner() != 1234 || info.Group() != 5678 {
		t.Error("Wrong owner/group")
	}
}

func TestFakeFSRead(t *testing.T) {
	// Test some basic aspects of the fakefs

	fs := newFakeFilesystem("/foo/bar/baz")

	// Create
	fd, _ := fs.Create("test")
	fd.Truncate(3 * 1 << randomBlockShift)

	// Read
	fd.Seek(0, io.SeekStart)
	bs0, err := ioutil.ReadAll(fd)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs0) != 3*1<<randomBlockShift {
		t.Error("wrong number of bytes:", len(bs0))
	}

	// Read again, starting at an odd offset
	fd.Seek(0, io.SeekStart)
	buf0 := make([]byte, 12345)
	n, _ := fd.Read(buf0)
	if n != len(buf0) {
		t.Fatal("short read")
	}
	buf1, err := ioutil.ReadAll(fd)
	if err != nil {
		t.Fatal(err)
	}
	if len(buf1) != 3*1<<randomBlockShift-len(buf0) {
		t.Error("wrong number of bytes:", len(buf1))
	}

	bs1 := append(buf0, buf1...)
	if !bytes.Equal(bs0, bs1) {
		t.Error("data mismatch")
	}

	// Read large block with ReadAt
	bs2 := make([]byte, 3*1<<randomBlockShift)
	_, err = fd.ReadAt(bs2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bs0, bs2) {
		t.Error("data mismatch")
	}
}

func TestFakeFSCaseInsensitive(t *testing.T) {
	fs := newFakeFilesystem("/foo/bar?insens=true")

	bs1 := []byte("test")

	err := fs.Mkdir("/fUbar", 0755)
	if err != nil {
		t.Fatal(err)
	}

	// "ΣΊΣΥΦΟΣ" and "Σίσυφος" denote the same file on OS X
	fd1, err := fs.Create("fuBAR/ΣΊΣΥΦΟΣ")
	if err != nil {
		t.Fatalf("could not create file: %s", err)
	}

	_, err = fd1.Write(bs1)
	if err != nil {
		t.Fatal(err)
	}

	// Try reading from the same file with different filenames
	fd2, err := fs.Open("Fubar/Σίσυφος")
	if err != nil {
		t.Fatalf("could not open file by its case-differing filename: %s", err)
	}

	fd2.Seek(0, io.SeekStart)

	bs2, err := ioutil.ReadAll(fd2)
	if err != nil {
		t.Fatal(err)
	}

	if len(bs1) != len(bs2) {
		t.Errorf("wrong number of bytes, expected %d, got %d", len(bs1), len(bs2))
	}

	// fd.Stat and fs.Stat should return the same name it was called for, not the actual filename
	info, err := fd2.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if info.Name() != "Σίσυφος" {
		t.Error("wrong name:", info.Name())
	}

	if info.Size() != 4 {
		t.Error("wrong size:", info.Size())
	}

	info, err = fs.Stat("fubar/σίσυφοσ")
	if err != nil {
		t.Fatal(err)
	}

	if info.Name() != "σίσυφοσ" {
		t.Error("wrong name:", info.Name())
	}

	if info.Size() != 4 {
		t.Error("wrong size:", info.Size())
	}
}

func TestFakeFSCaseInsensitiveMkdirAll(t *testing.T) {
	fs := newFakeFilesystem("/foo?insens=true")

	err := fs.MkdirAll("/fOO/Bar/bAz", 0755)
	if err != nil {
		t.Fatal(err)
	}

	fd, err := fs.OpenFile("/foo/BaR/BaZ/tESt", os.O_CREATE, 0644)
	if err != nil {
		t.Fatal(err)
	}

	if err = fd.Close(); err != nil {
		t.Fatal(err)
	}

	if err = fs.Rename("/FOO/BAR/baz/tesT", "/foo/baR/BAZ/TEst"); err != nil {
		t.Fatal(err)
	}
}

func TestFakeFSDirNames(t *testing.T) {
	fs := newFakeFilesystem("/")
	testDirNames(t, fs)

	fs = newFakeFilesystem("/?insens=true")
	testDirNames(t, fs)
}

func testDirNames(t *testing.T, fs *fakefs) {
	t.Helper()

	filenames := []string{"fOO", "Bar", "baz"}
	for _, filename := range filenames {
		if _, err := fs.Create("/" + filename); err != nil {
			t.Fatalf("Could not create %s: %s", filename, err)
		}
	}

	assertDir(t, fs, "/", filenames)
}

func assertDir(t *testing.T, fs *fakefs, directory string, filenames []string) {
	t.Helper()

	got, err := fs.DirNames(directory)
	if err != nil {
		t.Fatal(err)
	}

	if path.Clean(directory) == "/" {
		filenames = append(filenames, ".stfolder")
	}
	sort.Strings(filenames)
	sort.Strings(got)

	if !reflect.DeepEqual(got, filenames) {
		t.Errorf("want %s, got %s", filenames, got)
	}
}

func TestFakeFSStatIgnoreCase(t *testing.T) {
	fs := newFakeFilesystem("/foobaar?insens=true")

	if err := fs.Mkdir("/foo", 0755); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Create("/Foo/aaa"); err != nil {
		t.Fatal(err)
	}

	info, err := fs.Stat("/FOO/AAA")
	if err != nil {
		t.Fatal(err)
	}

	if _, err = fs.Stat("/fOO/aAa"); err != nil {
		t.Fatal(err)
	}

	if info.Name() != "AAA" {
		t.Errorf("want AAA, got %s", info.Name())
	}

	fd1, err := fs.Open("/FOO/AAA")
	if err != nil {
		t.Fatal(err)
	}

	if info, err = fd1.Stat(); err != nil {
		t.Fatal(err)
	}

	fd2, err := fs.Open("Foo/aAa")
	if err != nil {
		t.Fatal(err)
	}

	if _, err = fd2.Stat(); err != nil {
		t.Fatal(err)
	}

	if info.Name() != "AAA" {
		t.Errorf("want AAA, got %s", info.Name())
	}

	assertDir(t, fs, "/", []string{"foo"})
	assertDir(t, fs, "/foo", []string{"aaa"})
}

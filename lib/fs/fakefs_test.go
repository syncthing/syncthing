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

	if _, err := fd2.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	bs2, err := ioutil.ReadAll(fd2)
	if err != nil {
		t.Fatal(err)
	}

	if len(bs1) != len(bs2) {
		t.Errorf("wrong number of bytes, expected %d, got %d", len(bs1), len(bs2))
	}
}

func TestFakeFSCaseInsensitiveMkdirAll(t *testing.T) {
	fs := newFakeFilesystem("/fooi?insens=true")

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
	fs := newFakeFilesystem("/fbr")
	testDirNames(t, fs)

	fs = newFakeFilesystem("/fbri?insens=true")
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

func TestFakeFSFileName(t *testing.T) {
	var testCases = []struct {
		fs     string
		create string
		open   string
	}{
		{"/foo", "bar", "bar"},
		{"/fo?insens=true", "BaZ", "bAz"},
	}

	for _, testCase := range testCases {
		fs := newFakeFilesystem(testCase.fs)
		if _, err := fs.Create(testCase.create); err != nil {
			t.Fatal(err)
		}

		fd, err := fs.Open(testCase.open)
		if err != nil {
			t.Fatal(err)
		}

		if got := fd.Name(); got != testCase.open {
			t.Errorf("want %s, got %s", testCase.open, got)
		}
	}
}

func TestFakeFSRename(t *testing.T) {
	fs := newFakeFilesystem("/qux")
	if err := fs.MkdirAll("/foo/bar/baz", 0755); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Create("/foo/bar/baz/qux"); err != nil {
		t.Fatal(err)
	}

	if err := fs.Rename("/foo/bar/baz/qux", "/foo/baz/bar/qux"); err == nil {
		t.Errorf("rename to non-existent dir gave no error")
	}

	if err := fs.MkdirAll("/baz/bar/foo", 0755); err != nil {
		t.Fatal(err)
	}

	if err := fs.Rename("/foo/bar/baz/qux", "/baz/bar/foo/qux"); err != nil {
		t.Fatal(err)
	}

	var dirs = []struct {
		dir   string
		files []string
	}{
		{dir: "/", files: []string{"foo", "baz"}},
		{dir: "/foo", files: []string{"bar"}},
		{dir: "/foo/bar/baz", files: []string{}},
		{dir: "/baz/bar/foo", files: []string{"qux"}},
	}

	for _, dir := range dirs {
		assertDir(t, fs, dir.dir, dir.files)
	}

	if err := fs.Rename("/baz/bar/foo", "/baz/bar/FOO"); err != nil {
		t.Fatal(err)
	}

	assertDir(t, fs, "/baz/bar", []string{"FOO"})
	assertDir(t, fs, "/baz/bar/FOO", []string{"qux"})
}

func TestFakeFSRenameInsensitive(t *testing.T) {
	fs := newFakeFilesystem("/quux?insens=true")

	if err := fs.MkdirAll("/baz/bar/foo", 0755); err != nil {
		t.Fatal(err)
	}

	if err := fs.MkdirAll("/foO/baR/baZ", 0755); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Create("/BAZ/BAR/FOO/QUX"); err != nil {
		t.Fatal(err)
	}

	if err := fs.Rename("/Baz/bAr/foO/QuX", "/Foo/Bar/Baz/qUUx"); err != nil {
		t.Fatal(err)
	}

	var dirs = []struct {
		dir   string
		files []string
	}{
		{dir: "/", files: []string{"foO", "baz"}},
		{dir: "/foo", files: []string{"baR"}},
		{dir: "/foo/bar/baz", files: []string{"qUUx"}},
		{dir: "/baz/bar/foo", files: []string{}},
	}

	for _, dir := range dirs {
		assertDir(t, fs, dir.dir, dir.files)
	}

	if err := fs.Rename("/foo/bar/BAZ", "/FOO/BAR/bAz"); err != nil {
		t.Fatal(err)
	}

	assertDir(t, fs, "/Foo/Bar", []string{"bAz"})
	assertDir(t, fs, "/fOO/bAr/baz", []string{"qUUx"})

	if err := fs.Rename("foo/bar/baz/quux", "foo/bar/baz/Quux"); err != nil {
		t.Fatal(err)
	}

	assertDir(t, fs, "/FOO/BAR/BAZ", []string{"Quux"})
}

func TestFakeFSMkdir(t *testing.T) {
	fs := newFakeFilesystem("/mkdir")

	if err := fs.Mkdir("/foo", 0755); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Stat("/foo"); err != nil {
		t.Fatal(err)
	}

	if err := fs.Mkdir("/foo", 0755); err == nil {
		t.Errorf("got no error while creating existing directory")
	}

	fs = newFakeFilesystem("/mkdiri?insens=true")

	if err := fs.Mkdir("/foo", 0755); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Stat("/Foo"); err != nil {
		t.Fatal(err)
	}

	if err := fs.Mkdir("/FOO", 0755); err == nil {
		t.Errorf("got no error while creating existing directory")
	}
}

func TestFakeFSOpenFile(t *testing.T) {
	fs := newFakeFilesystem("/openf")

	if _, err := fs.OpenFile("foobar", os.O_RDONLY, 0664); err == nil {
		t.Errorf("got no error opening a non-existing file")
	}

	if _, err := fs.OpenFile("foobar", os.O_RDWR|os.O_CREATE, 0664); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.OpenFile("foobar", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0664); err == nil {
		t.Errorf("created an existing file while told not to")
	}

	if _, err := fs.OpenFile("foobar", os.O_RDWR|os.O_CREATE, 0664); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.OpenFile("foobar", os.O_RDWR, 0664); err != nil {
		t.Fatal(err)
	}
}

func TestFakeFSOpenFileInsens(t *testing.T) {
	fs := newFakeFilesystem("/openfi?insens=true")

	if _, err := fs.OpenFile("FooBar", os.O_RDONLY, 0664); err == nil {
		t.Errorf("got no error opening a non-existing file")
	}

	if _, err := fs.OpenFile("fOObar", os.O_RDWR|os.O_CREATE, 0664); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.OpenFile("fOoBaR", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0664); err == nil {
		t.Errorf("created an existing file while told not to")
	}

	if _, err := fs.OpenFile("FoObAr", os.O_RDWR|os.O_CREATE, 0664); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.OpenFile("FOOBAR", os.O_RDWR, 0664); err != nil {
		t.Fatal(err)
	}
}

func TestFakeFSRemoveAll(t *testing.T) {
	fs := newFakeFilesystem("/removeall")

	if err := fs.Mkdir("/foo", 0755); err != nil {
		t.Fatal(err)
	}

	filenames := []string{"bar", "baz", "qux"}
	for _, filename := range filenames {
		if _, err := fs.Create("/foo/" + filename); err != nil {
			t.Fatalf("Could not create %s: %s", filename, err)
		}
	}

	if err := fs.RemoveAll("/foo"); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Stat("/foo"); err == nil {
		t.Errorf("this should not exist anymore")
	}

	if err := fs.RemoveAll("/foo/bar"); err == nil {
		t.Errorf("should have returned error")
	}
}

func TestFakeFSRemoveAllInsens(t *testing.T) {
	fs := newFakeFilesystem("/removealli?insens=true")

	if err := fs.Mkdir("/Foo", 0755); err != nil {
		t.Fatal(err)
	}

	filenames := []string{"bar", "baz", "qux"}
	for _, filename := range filenames {
		if _, err := fs.Create("/FOO/" + filename); err != nil {
			t.Fatalf("Could not create %s: %s", filename, err)
		}
	}

	if err := fs.RemoveAll("/fOo"); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Stat("/foo"); err == nil {
		t.Errorf("this should not exist anymore")
	}

	if err := fs.RemoveAll("/foO/bAr"); err == nil {
		t.Errorf("should have returned error")
	}
}

func TestFakeFSRemove(t *testing.T) {
	fs := newFakeFilesystem("/remove")

	if err := fs.Mkdir("/Foo", 0755); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Create("/Foo/Bar"); err != nil {
		t.Fatal(err)
	}

	if err := fs.Remove("/Foo"); err == nil {
		t.Errorf("not empty, should give error")
	}

	if err := fs.Remove("/Foo/Bar"); err != nil {
		t.Fatal(err)
	}

	if err := fs.Remove("/Foo"); err != nil {
		t.Fatal(err)
	}
}

func TestFakeFSRemoveInsens(t *testing.T) {
	fs := newFakeFilesystem("/removei?insens=true")

	if err := fs.Mkdir("/Foo", 0755); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Create("/Foo/Bar"); err != nil {
		t.Fatal(err)
	}

	if err := fs.Remove("/FOO"); err == nil || err == os.ErrNotExist {
		t.Errorf("not empty, should give error")
	}

	if err := fs.Remove("/Foo/BaR"); err != nil {
		t.Fatal(err)
	}

	if err := fs.Remove("/FoO"); err != nil {
		t.Fatal(err)
	}
}

func TestFakeFSSameFile(t *testing.T) {
	fs := newFakeFilesystem("/samefile")

	if err := fs.Mkdir("/Foo", 0755); err != nil {
		t.Fatal(err)
	}

	filenames := []string{"Bar", "Baz", "/Foo/Bar"}
	for _, filename := range filenames {
		if _, err := fs.Create(filename); err != nil {
			t.Fatalf("Could not create %s: %s", filename, err)
		}
	}

	testCases := []struct {
		f1   string
		f2   string
		want bool
	}{
		{f1: "Bar", f2: "Baz", want: false},
		{f1: "Bar", f2: "/Foo/Bar", want: true},
	}

	for _, test := range testCases {
		assertSameFile(t, fs, test.f1, test.f2, test.want)
	}
}

func TestFakeFSSameFileInsens(t *testing.T) {
	fs := newFakeFilesystem("/samefilei?insens=true")

	if err := fs.Mkdir("/Foo", 0755); err != nil {
		t.Fatal(err)
	}

	filenames := []string{"Bar", "Baz", "/Foo/BAR"}
	for _, filename := range filenames {
		if _, err := fs.Create(filename); err != nil {
			t.Fatalf("Could not create %s: %s", filename, err)
		}
	}

	testCases := []struct {
		f1   string
		f2   string
		want bool
	}{
		{f1: "bAr", f2: "baZ", want: false},
		{f1: "baR", f2: "/fOO/bAr", want: true},
	}

	for _, test := range testCases {
		assertSameFile(t, fs, test.f1, test.f2, test.want)
	}
}

func assertSameFile(t *testing.T, fs *fakefs, f1, f2 string, want bool) {
	t.Helper()

	fi1, err := fs.Stat(f1)
	if err != nil {
		t.Fatal(err)
	}

	fi2, err := fs.Stat(f2)
	if err != nil {
		t.Fatal(err)
	}

	got := fs.SameFile(fi1, fi2)
	if got != want {
		t.Errorf("for \"%s\" and \"%s\" want SameFile %v, got %v", f1, f2, want, got)
	}
}

func TestFakeFSCreateInsens(t *testing.T) {
	fs := newFakeFilesystem("/createi?insens=true")

	fd1, err := fs.Create("FOO")
	if err != nil {
		t.Fatal(err)
	}

	fd2, err := fs.Create("fOo")
	if err != nil {
		t.Fatal(err)
	}

	if fd2.Name() != "fOo" {
		t.Errorf("name of created file \"fOo\" is %s", fd2.Name())
	}

	if fd1.Name() != "fOo" {
		t.Errorf("name of the file created as \"FOO\" is %s", fd1.Name())
	}
}

// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/build"
)

func TestFakeFS(t *testing.T) {
	// Test some basic aspects of the fakeFS

	fs := newFakeFilesystem("/foo/bar/baz")

	// MkdirAll
	err := fs.MkdirAll("dira/dirb", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fs.Stat("dira/dirb")
	if err != nil {
		t.Fatal(err)
	}

	// Mkdir
	err = fs.Mkdir("dira/dirb/dirc", 0o755)
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
	bs0, err := io.ReadAll(fd)
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
	bs1, err := io.ReadAll(fd)
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
	if err := fs.Lchown("dira", "1234", "5678"); err != nil {
		t.Fatal(err)
	}
	if info, err := fs.Lstat("dira"); err != nil {
		t.Fatal(err)
	} else if info.Owner() != 1234 || info.Group() != 5678 {
		t.Error("Wrong owner/group")
	}
}

func testFakeFSRead(t *testing.T, fs Filesystem) {
	// Test some basic aspects of the fakeFS
	// Create
	fd, _ := fs.Create("test")
	defer fd.Close()
	fd.Truncate(3 * 1 << randomBlockShift)

	// Read
	fd.Seek(0, io.SeekStart)
	bs0, err := io.ReadAll(fd)
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
	buf1, err := io.ReadAll(fd)
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

type testFS struct {
	name string
	fs   Filesystem
}

type test struct {
	name string
	impl func(t *testing.T, fs Filesystem)
}

func TestFakeFSCaseSensitive(t *testing.T) {
	tests := []test{
		{"Read", testFakeFSRead},
		{"OpenFile", testFakeFSOpenFile},
		{"RemoveAll", testFakeFSRemoveAll},
		{"Remove", testFakeFSRemove},
		{"Rename", testFakeFSRename},
		{"Mkdir", testFakeFSMkdir},
		{"SameFile", testFakeFSSameFile},
		{"DirNames", testDirNames},
		{"FileName", testFakeFSFileName},
	}
	filesystems := []testFS{
		{"fakeFS", newFakeFilesystem("/foo")},
	}

	testDir, sensitive := createTestDir(t)
	if sensitive {
		filesystems = append(filesystems, testFS{runtime.GOOS, newBasicFilesystem(testDir)})
	}

	runTests(t, tests, filesystems)
}

func TestFakeFSCaseInsensitive(t *testing.T) {
	tests := []test{
		{"Read", testFakeFSRead},
		{"OpenFile", testFakeFSOpenFile},
		{"RemoveAll", testFakeFSRemoveAll},
		{"Remove", testFakeFSRemove},
		{"Mkdir", testFakeFSMkdir},
		{"SameFile", testFakeFSSameFile},
		{"DirNames", testDirNames},
		{"FileName", testFakeFSFileName},
		{"GeneralInsens", testFakeFSCaseInsensitive},
		{"MkdirAllInsens", testFakeFSCaseInsensitiveMkdirAll},
		{"StatInsens", testFakeFSStatInsens},
		{"RenameInsens", testFakeFSRenameInsensitive},
		{"MkdirInsens", testFakeFSMkdirInsens},
		{"OpenFileInsens", testFakeFSOpenFileInsens},
		{"RemoveAllInsens", testFakeFSRemoveAllInsens},
		{"RemoveInsens", testFakeFSRemoveInsens},
		{"SameFileInsens", testFakeFSSameFileInsens},
		{"CreateInsens", testFakeFSCreateInsens},
		{"FileNameInsens", testFakeFSFileNameInsens},
	}

	filesystems := []testFS{
		{"fakeFS", newFakeFilesystem("/foobar?insens=true")},
	}

	testDir, sensitive := createTestDir(t)
	if !sensitive {
		filesystems = append(filesystems, testFS{runtime.GOOS, newBasicFilesystem(testDir)})
	}

	runTests(t, tests, filesystems)
}

func createTestDir(t *testing.T) (string, bool) {
	t.Helper()

	testDir := t.TempDir()

	if fd, err := os.Create(filepath.Join(testDir, ".stfolder")); err != nil {
		t.Fatalf("could not create .stfolder: %s", err)
	} else {
		fd.Close()
	}

	var sensitive bool

	if f, err := os.Open(filepath.Join(testDir, ".STfolder")); err != nil {
		sensitive = true
	} else {
		defer f.Close()
	}

	return testDir, sensitive
}

func runTests(t *testing.T, tests []test, filesystems []testFS) {
	for _, filesystem := range filesystems {
		for _, test := range tests {
			name := fmt.Sprintf("%s_%s", test.name, filesystem.name)
			t.Run(name, func(t *testing.T) {
				test.impl(t, filesystem.fs)
				if err := cleanup(filesystem.fs); err != nil {
					t.Errorf("cleanup failed: %s", err)
				}
			})
		}
	}
}

func testFakeFSCaseInsensitive(t *testing.T, fs Filesystem) {
	bs1 := []byte("test")

	err := fs.Mkdir("/fUbar", 0o755)
	if err != nil {
		t.Fatal(err)
	}

	fd1, err := fs.Create("fuBAR/SISYPHOS")
	if err != nil {
		t.Fatalf("could not create file: %s", err)
	}

	defer fd1.Close()

	_, err = fd1.Write(bs1)
	if err != nil {
		t.Fatal(err)
	}

	// Try reading from the same file with different filenames
	fd2, err := fs.Open("Fubar/Sisyphos")
	if err != nil {
		t.Fatalf("could not open file by its case-differing filename: %s", err)
	}

	defer fd2.Close()

	if _, err := fd2.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	bs2, err := io.ReadAll(fd2)
	if err != nil {
		t.Fatal(err)
	}

	if len(bs1) != len(bs2) {
		t.Errorf("wrong number of bytes, expected %d, got %d", len(bs1), len(bs2))
	}
}

func testFakeFSCaseInsensitiveMkdirAll(t *testing.T, fs Filesystem) {
	err := fs.MkdirAll("/fOO/Bar/bAz", 0o755)
	if err != nil {
		t.Fatal(err)
	}

	fd, err := fs.OpenFile("/foo/BaR/BaZ/tESt", os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	if err = fd.Close(); err != nil {
		t.Fatal(err)
	}

	if err = fs.Rename("/FOO/BAR/baz/tesT", "/foo/baR/BAZ/Qux"); err != nil {
		t.Fatal(err)
	}
}

func testDirNames(t *testing.T, fs Filesystem) {
	filenames := []string{"fOO", "Bar", "baz"}
	for _, filename := range filenames {
		if fd, err := fs.Create("/" + filename); err != nil {
			t.Errorf("Could not create %s: %s", filename, err)
		} else {
			fd.Close()
		}
	}

	assertDir(t, fs, "/", filenames)
}

func assertDir(t *testing.T, fs Filesystem, directory string, filenames []string) {
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

	if len(filenames) != len(got) {
		t.Errorf("want %s, got %s", filenames, got)
		return
	}

	for i := range filenames {
		if filenames[i] != got[i] {
			t.Errorf("want %s, got %s", filenames, got)
			return
		}
	}
}

func testFakeFSStatInsens(t *testing.T, fs Filesystem) {
	// this is to test that neither fs.Stat nor fd.Stat change the filename
	// both in directory and in previous Stat results
	fd1, err := fs.Create("aAa")
	if err != nil {
		t.Fatal(err)
	}
	defer fd1.Close()

	info1, err := fs.Stat("AAA")
	if err != nil {
		t.Fatal(err)
	}

	if _, err = fs.Stat("AaA"); err != nil {
		t.Fatal(err)
	}

	info2, err := fd1.Stat()
	if err != nil {
		t.Fatal(err)
	}

	fd2, err := fs.Open("aaa")
	if err != nil {
		t.Fatal(err)
	}
	defer fd2.Close()

	if _, err = fd2.Stat(); err != nil {
		t.Fatal(err)
	}

	if info1.Name() != "AAA" {
		t.Errorf("want AAA, got %s", info1.Name())
	}

	if info2.Name() != "aAa" {
		t.Errorf("want aAa, got %s", info2.Name())
	}

	assertDir(t, fs, "/", []string{"aAa"})
}

func testFakeFSFileName(t *testing.T, fs Filesystem) {
	testCases := []struct {
		create string
		open   string
	}{
		{"bar", "bar"},
	}

	for _, testCase := range testCases {
		if fd, err := fs.Create(testCase.create); err != nil {
			t.Fatal(err)
		} else {
			fd.Close()
		}

		fd, err := fs.Open(testCase.open)
		if err != nil {
			t.Fatal(err)
		}

		defer fd.Close()

		if got := fd.Name(); got != testCase.open {
			t.Errorf("want %s, got %s", testCase.open, got)
		}
	}
}

func testFakeFSFileNameInsens(t *testing.T, fs Filesystem) {
	testCases := []struct {
		create string
		open   string
	}{
		{"BaZ", "bAz"},
	}

	for _, testCase := range testCases {
		fd, err := fs.Create(testCase.create)
		if err != nil {
			t.Fatal(err)
		}
		fd.Close()

		fd, err = fs.Open(testCase.open)
		if err != nil {
			t.Fatal(err)
		}

		defer fd.Close()

		if got := fd.Name(); got != testCase.open {
			t.Errorf("want %s, got %s", testCase.open, got)
		}
	}
}

func testFakeFSRename(t *testing.T, fs Filesystem) {
	if err := fs.MkdirAll("/foo/bar/baz", 0o755); err != nil {
		t.Fatal(err)
	}

	fd, err := fs.Create("/foo/bar/baz/qux")
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	if err := fs.Rename("/foo/bar/baz/qux", "/foo/notthere/qux"); err == nil {
		t.Errorf("rename to non-existent dir gave no error")
	}

	if err := fs.MkdirAll("/baz/bar/foo", 0o755); err != nil {
		t.Fatal(err)
	}

	if err := fs.Rename("/foo/bar/baz/qux", "/baz/bar/foo/qux"); err != nil {
		t.Fatal(err)
	}

	dirs := []struct {
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

func testFakeFSRenameInsensitive(t *testing.T, fs Filesystem) {
	if err := fs.MkdirAll("/baz/bar/foo", 0o755); err != nil {
		t.Fatal(err)
	}

	if err := fs.MkdirAll("/foO/baR/baZ", 0o755); err != nil {
		t.Fatal(err)
	}

	fd, err := fs.Create("/BAZ/BAR/FOO/QUX")
	if err != nil {
		t.Fatal(err)
	}

	fd.Close()

	if err := fs.Rename("/Baz/bAr/foO/QuX", "/Foo/Bar/Baz/qUUx"); err != nil {
		t.Fatal(err)
	}

	dirs := []struct {
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

	// not checking on darwin due to https://github.com/golang/go/issues/35222
	if !build.IsDarwin {
		if err := fs.Rename("/foo/bar/BAZ", "/FOO/BAR/bAz"); err != nil {
			t.Errorf("Could not perform in-place case-only directory rename: %s", err)
		}

		assertDir(t, fs, "/foo/bar", []string{"bAz"})
		assertDir(t, fs, "/fOO/bAr/baz", []string{"qUUx"})
	}

	if err := fs.Rename("foo/bar/baz/quux", "foo/bar/BaZ/Quux"); err != nil {
		t.Errorf("File rename failed: %s", err)
	}

	assertDir(t, fs, "/FOO/BAR/BAZ", []string{"Quux"})
}

func testFakeFSMkdir(t *testing.T, fs Filesystem) {
	if err := fs.Mkdir("/foo", 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Stat("/foo"); err != nil {
		t.Fatal(err)
	}

	if err := fs.Mkdir("/foo", 0o755); err == nil {
		t.Errorf("got no error while creating existing directory")
	}
}

func testFakeFSMkdirInsens(t *testing.T, fs Filesystem) {
	if err := fs.Mkdir("/foo", 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Stat("/Foo"); err != nil {
		t.Fatal(err)
	}

	if err := fs.Mkdir("/FOO", 0o755); err == nil {
		t.Errorf("got no error while creating existing directory")
	}
}

func testFakeFSOpenFile(t *testing.T, fs Filesystem) {
	fd, err := fs.OpenFile("foobar", os.O_RDONLY, 0o664)
	if err == nil {
		fd.Close()
		t.Fatalf("got no error opening a non-existing file")
	}

	fd, err = fs.OpenFile("foobar", os.O_RDWR|os.O_CREATE, 0o664)
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	fd, err = fs.OpenFile("foobar", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o664)
	if err == nil {
		fd.Close()
		t.Fatalf("created an existing file while told not to")
	}

	fd, err = fs.OpenFile("foobar", os.O_RDWR|os.O_CREATE, 0o664)
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	fd, err = fs.OpenFile("foobar", os.O_RDWR, 0o664)
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()
}

func testFakeFSOpenFileInsens(t *testing.T, fs Filesystem) {
	fd, err := fs.OpenFile("FooBar", os.O_RDONLY, 0o664)
	if err == nil {
		fd.Close()
		t.Fatalf("got no error opening a non-existing file")
	}

	fd, err = fs.OpenFile("fOObar", os.O_RDWR|os.O_CREATE, 0o664)
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	fd, err = fs.OpenFile("fOoBaR", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o664)
	if err == nil {
		fd.Close()
		t.Fatalf("created an existing file while told not to")
	}

	fd, err = fs.OpenFile("FoObAr", os.O_RDWR|os.O_CREATE, 0o664)
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	fd, err = fs.OpenFile("FOOBAR", os.O_RDWR, 0o664)
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()
}

func testFakeFSRemoveAll(t *testing.T, fs Filesystem) {
	if err := fs.Mkdir("/foo", 0o755); err != nil {
		t.Fatal(err)
	}

	filenames := []string{"bar", "baz", "qux"}
	for _, filename := range filenames {
		fd, err := fs.Create("/foo/" + filename)
		if err != nil {
			t.Fatalf("Could not create %s: %s", filename, err)
		} else {
			fd.Close()
		}
	}

	if err := fs.RemoveAll("/foo"); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Stat("/foo"); err == nil {
		t.Errorf("this should be an error, as file doesn not exist anymore")
	}

	if err := fs.RemoveAll("/foo/bar"); err != nil {
		t.Errorf("real systems don't return error here")
	}
}

func testFakeFSRemoveAllInsens(t *testing.T, fs Filesystem) {
	if err := fs.Mkdir("/Foo", 0o755); err != nil {
		t.Fatal(err)
	}

	filenames := []string{"bar", "baz", "qux"}
	for _, filename := range filenames {
		fd, err := fs.Create("/FOO/" + filename)
		if err != nil {
			t.Fatalf("Could not create %s: %s", filename, err)
		}
		fd.Close()
	}

	if err := fs.RemoveAll("/fOo"); err != nil {
		t.Errorf("Could not remove dir: %s", err)
	}

	if _, err := fs.Stat("/foo"); err == nil {
		t.Errorf("this should be an error, as file doesn not exist anymore")
	}

	if err := fs.RemoveAll("/foO/bAr"); err != nil {
		t.Errorf("real systems don't return error here")
	}
}

func testFakeFSRemove(t *testing.T, fs Filesystem) {
	if err := fs.Mkdir("/Foo", 0o755); err != nil {
		t.Fatal(err)
	}

	fd, err := fs.Create("/Foo/Bar")
	if err != nil {
		t.Fatal(err)
	} else {
		fd.Close()
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

func testFakeFSRemoveInsens(t *testing.T, fs Filesystem) {
	if err := fs.Mkdir("/Foo", 0o755); err != nil {
		t.Fatal(err)
	}

	fd, err := fs.Create("/Foo/Bar")
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	if err := fs.Remove("/FOO"); err == nil {
		t.Errorf("not empty, should give error")
	}

	if err := fs.Remove("/Foo/BaR"); err != nil {
		t.Fatal(err)
	}

	if err := fs.Remove("/FoO"); err != nil {
		t.Fatal(err)
	}
}

func testFakeFSSameFile(t *testing.T, fs Filesystem) {
	if err := fs.Mkdir("/Foo", 0o755); err != nil {
		t.Fatal(err)
	}

	filenames := []string{"Bar", "Baz", "/Foo/Bar"}
	for _, filename := range filenames {
		if fd, err := fs.Create(filename); err != nil {
			t.Fatalf("Could not create %s: %s", filename, err)
		} else {
			fd.Close()
			if build.IsWindows {
				time.Sleep(1 * time.Millisecond)
			}
		}
	}

	testCases := []struct {
		f1   string
		f2   string
		want bool
	}{
		{"Bar", "Baz", false},
		{"Bar", "/Foo/Bar", false},
		{"Bar", "Bar", true},
	}

	for _, test := range testCases {
		assertSameFile(t, fs, test.f1, test.f2, test.want)
	}
}

func testFakeFSSameFileInsens(t *testing.T, fs Filesystem) {
	if err := fs.Mkdir("/Foo", 0o755); err != nil {
		t.Fatal(err)
	}

	filenames := []string{"Bar", "Baz"}
	for _, filename := range filenames {
		fd, err := fs.Create(filename)
		if err != nil {
			t.Errorf("Could not create %s: %s", filename, err)
		}
		fd.Close()
	}

	testCases := []struct {
		f1   string
		f2   string
		want bool
	}{
		{"bAr", "baZ", false},
		{"baz", "BAZ", true},
	}

	for _, test := range testCases {
		assertSameFile(t, fs, test.f1, test.f2, test.want)
	}
}

func assertSameFile(t *testing.T, fs Filesystem, f1, f2 string, want bool) {
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

func testFakeFSCreateInsens(t *testing.T, fs Filesystem) {
	fd1, err := fs.Create("FOO")
	if err != nil {
		t.Fatal(err)
	}

	defer fd1.Close()

	fd2, err := fs.Create("fOo")
	if err != nil {
		t.Fatal(err)
	}

	defer fd2.Close()

	if fd1.Name() != "FOO" {
		t.Errorf("name of the file created as \"FOO\" is %s", fd1.Name())
	}

	if fd2.Name() != "fOo" {
		t.Errorf("name of created file \"fOo\" is %s", fd2.Name())
	}

	// one would expect DirNames to show the last wrapperType, but in fact it shows
	// the original one
	assertDir(t, fs, "/", []string{"FOO"})
}

func TestReadWriteContent(t *testing.T) {
	fs := newFakeFilesystem("foo?content=true")
	fd, err := fs.Create("file")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := fd.Write([]byte("foo")); err != nil {
		t.Fatal(err)
	}
	if _, err := fd.WriteAt([]byte("bar"), 5); err != nil {
		t.Fatal(err)
	}
	expected := []byte("foo\x00\x00bar")

	buf := make([]byte, len(expected)-1)
	n, err := fd.ReadAt(buf, 1) // note offset one byte
	if err != nil {
		t.Fatal(err)
	}
	if n != len(expected)-1 {
		t.Fatal("wrong number of bytes read")
	}
	if !bytes.Equal(buf[:n], expected[1:]) {
		fmt.Printf("%d %q\n", n, buf[:n])
		t.Error("wrong data in file")
	}
}

func cleanup(fs Filesystem) error {
	filenames, _ := fs.DirNames("/")
	for _, filename := range filenames {
		if filename != ".stfolder" {
			if err := fs.RemoveAll(filename); err != nil {
				return err
			}
		}
	}

	return nil
}

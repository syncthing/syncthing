// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/osutil"
)

func TestTraversesSymlink(t *testing.T) {
	tmpDir, err := os.MkdirTemp(".", ".test-TraversesSymlink-")
	if err != nil {
		panic("Failed to create temporary testing dir")
	}
	defer os.RemoveAll(tmpDir)

	testFs := fs.NewFilesystem(fs.FilesystemTypeBasic, tmpDir)
	testFs.MkdirAll("a/b/c", 0755)
	if err = fs.DebugSymlinkForTestsOnly(testFs, testFs, filepath.Join("a", "b"), filepath.Join("a", "l")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("Symlinks aren't working")
		}
		t.Fatal(err)
	}

	// a/l -> b, so a/l/c should resolve by normal stat
	info, err := testFs.Lstat("a/l/c")
	if err != nil {
		t.Fatal("unexpected error", err)
	}
	if !info.IsDir() {
		t.Fatal("error in setup, a/l/c should be a directory")
	}

	cases := []struct {
		name      string
		traverses bool
	}{
		// Exist
		{".", false},
		{"a", false},
		{"a/b", false},
		{"a/b/c", false},
		// Don't exist
		{"x", false},
		{"a/x", false},
		{"a/b/x", false},
		{"a/x/c", false},
		// Symlink or behind symlink
		{"a/l", true},
		{"a/l/c", true},
		// Non-existing behind a symlink
		{"a/l/x", true},
	}

	for _, tc := range cases {
		if res := osutil.TraversesSymlink(testFs, tc.name); tc.traverses == (res == nil) {
			t.Errorf("TraversesSymlink(%q) = %v, should be %v", tc.name, res, tc.traverses)
		}
	}
}

func TestIssue4875(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", ".test-Issue4875-")
	if err != nil {
		panic("Failed to create temporary testing dir")
	}
	defer os.RemoveAll(tmpDir)

	testFs := fs.NewFilesystem(fs.FilesystemTypeBasic, tmpDir)
	testFs.MkdirAll(filepath.Join("a", "b", "c"), 0755)
	if err = fs.DebugSymlinkForTestsOnly(testFs, testFs, filepath.Join("a", "b"), filepath.Join("a", "l")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("Symlinks aren't working")
		}
		t.Fatal(err)
	}

	// a/l -> b, so a/l/c should resolve by normal stat
	info, err := testFs.Lstat("a/l/c")
	if err != nil {
		t.Fatal("unexpected error", err)
	}
	if !info.IsDir() {
		t.Fatal("error in setup, a/l/c should be a directory")
	}

	testFs = fs.NewFilesystem(fs.FilesystemTypeBasic, filepath.Join(tmpDir, "a/l"))
	if err := osutil.TraversesSymlink(testFs, "."); err != nil {
		t.Error(`TraversesSymlink on filesystem with symlink at root returned error for ".":`, err)
	}
}

var traversesSymlinkResult error

func BenchmarkTraversesSymlink(b *testing.B) {
	os.RemoveAll("testdata")
	defer os.RemoveAll("testdata")
	fs := fs.NewFilesystem(fs.FilesystemTypeBasic, "testdata")
	fs.MkdirAll("a/b/c", 0755)

	for i := 0; i < b.N; i++ {
		traversesSymlinkResult = osutil.TraversesSymlink(fs, "a/b/c")
	}

	b.ReportAllocs()
}

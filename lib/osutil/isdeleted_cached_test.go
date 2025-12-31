// Copyright (C) 2025 The Syncthing Authors & bxff
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/

package osutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/osutil"
)

// syscallCountingFS wraps a filesystem and counts Lstat and DirNames calls
type syscallCountingFS struct {
	fs.Filesystem
	LstatCount    int
	DirNamesCount int
}

func (s *syscallCountingFS) Lstat(name string) (fs.FileInfo, error) {
	s.LstatCount++
	return s.Filesystem.Lstat(name)
}

func (s *syscallCountingFS) DirNames(name string) ([]string, error) {
	s.DirNamesCount++
	return s.Filesystem.DirNames(name)
}

func (s *syscallCountingFS) Reset() {
	s.LstatCount = 0
	s.DirNamesCount = 0
}

func (s *syscallCountingFS) TotalSyscalls() int {
	return s.LstatCount + s.DirNamesCount
}

// TestIsDeletedCachedCorrectness verifies that IsDeletedCached returns
// the same results as IsDeleted for all files
func TestIsDeletedCachedCorrectness(t *testing.T) {
	// Use a temp directory with test files
	tmpDir := t.TempDir()

	// Create test file structure
	dirs := []string{"a", "a/b", "a/b/c", "x/y/z"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(tmpDir, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	files := []string{"a/file1.txt", "a/b/file2.txt", "a/b/c/file3.txt", "x/y/z/file4.txt"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, f), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	baseFs := fs.NewFilesystem(fs.FilesystemTypeBasic, tmpDir)
	countingFs := &syscallCountingFS{Filesystem: baseFs}

	// Test existing files
	for _, f := range files {
		origResult := osutil.IsDeleted(countingFs, f)

		dirCache := osutil.NewDirExistenceCache(countingFs)
		symlinkCache := osutil.NewSymlinkCache(countingFs)
		cachedResult := osutil.IsDeletedCached(countingFs, f, dirCache, symlinkCache)

		if origResult != cachedResult {
			t.Errorf("IsDeletedCached(%q) = %v, want %v (same as IsDeleted)", f, cachedResult, origResult)
		}
	}

	// Test non-existing files
	nonExisting := []string{"a/missing.txt", "nonexistent/path/file.txt"}
	for _, f := range nonExisting {
		origResult := osutil.IsDeleted(countingFs, f)

		dirCache := osutil.NewDirExistenceCache(countingFs)
		symlinkCache := osutil.NewSymlinkCache(countingFs)
		cachedResult := osutil.IsDeletedCached(countingFs, f, dirCache, symlinkCache)

		if origResult != cachedResult {
			t.Errorf("IsDeletedCached(%q) = %v, want %v (same as IsDeleted)", f, cachedResult, origResult)
		}
	}
}

// TestIsDeletedCachedReducesSyscalls verifies that IsDeletedCached makes
// significantly fewer syscalls than IsDeleted when checking multiple files
func TestIsDeletedCachedReducesSyscalls(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a deeper directory structure with many files
	for i := 0; i < 10; i++ {
		dir := filepath.Join(tmpDir, "level1", "level2", "level3", "level4")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Create 100 files in the same deep directory
	deepDir := filepath.Join(tmpDir, "level1", "level2", "level3", "level4")
	var files []string
	for i := 0; i < 100; i++ {
		fname := filepath.Join("level1", "level2", "level3", "level4", "file"+string(rune('0'+i/10))+string(rune('0'+i%10))+".txt")
		fullPath := filepath.Join(tmpDir, fname)
		if err := os.WriteFile(fullPath, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
		files = append(files, fname)
	}
	_ = deepDir

	baseFs := fs.NewFilesystem(fs.FilesystemTypeBasic, tmpDir)

	// Measure original IsDeleted
	origFs := &syscallCountingFS{Filesystem: baseFs}
	for _, f := range files {
		osutil.IsDeleted(origFs, f)
	}
	origSyscalls := origFs.TotalSyscalls()

	// Measure cached IsDeletedCached
	cachedFs := &syscallCountingFS{Filesystem: baseFs}
	dirCache := osutil.NewDirExistenceCache(cachedFs)
	symlinkCache := osutil.NewSymlinkCache(cachedFs)
	for _, f := range files {
		osutil.IsDeletedCached(cachedFs, f, dirCache, symlinkCache)
	}
	cachedSyscalls := cachedFs.TotalSyscalls()

	t.Logf("Original IsDeleted:  %d syscalls for %d files (%.2f per file)",
		origSyscalls, len(files), float64(origSyscalls)/float64(len(files)))
	t.Logf("Cached IsDeletedCached: %d syscalls for %d files (%.2f per file)",
		cachedSyscalls, len(files), float64(cachedSyscalls)/float64(len(files)))
	t.Logf("Reduction: %.1fx fewer syscalls", float64(origSyscalls)/float64(cachedSyscalls))

	// The cached version should use significantly fewer syscalls
	if cachedSyscalls >= origSyscalls/5 {
		t.Errorf("Expected at least 5x syscall reduction, got %.1fx", float64(origSyscalls)/float64(cachedSyscalls))
	}
}

// BenchmarkIsDeleted benchmarks the original IsDeleted function
func BenchmarkIsDeleted(b *testing.B) {
	tmpDir := b.TempDir()

	// Create test structure
	dir := filepath.Join(tmpDir, "a", "b", "c", "d")
	if err := os.MkdirAll(dir, 0755); err != nil {
		b.Fatal(err)
	}
	testFile := filepath.Join("a", "b", "c", "d", "file.txt")
	if err := os.WriteFile(filepath.Join(tmpDir, testFile), []byte("test"), 0644); err != nil {
		b.Fatal(err)
	}

	baseFs := fs.NewFilesystem(fs.FilesystemTypeBasic, tmpDir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		osutil.IsDeleted(baseFs, testFile)
	}
}

// BenchmarkIsDeletedCached benchmarks the cached IsDeletedCached function
func BenchmarkIsDeletedCached(b *testing.B) {
	tmpDir := b.TempDir()

	// Create test structure
	dir := filepath.Join(tmpDir, "a", "b", "c", "d")
	if err := os.MkdirAll(dir, 0755); err != nil {
		b.Fatal(err)
	}
	testFile := filepath.Join("a", "b", "c", "d", "file.txt")
	if err := os.WriteFile(filepath.Join(tmpDir, testFile), []byte("test"), 0644); err != nil {
		b.Fatal(err)
	}

	baseFs := fs.NewFilesystem(fs.FilesystemTypeBasic, tmpDir)
	dirCache := osutil.NewDirExistenceCache(baseFs)
	symlinkCache := osutil.NewSymlinkCache(baseFs)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		osutil.IsDeletedCached(baseFs, testFile, dirCache, symlinkCache)
	}
}

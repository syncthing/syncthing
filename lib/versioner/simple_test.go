// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	stdfs "io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
)

func makeTreeOwnerWritable(t *testing.T, root string) {
	t.Helper()

	_ = filepath.WalkDir(root, func(path string, d stdfs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			_ = os.Chmod(path, info.Mode().Perm()|0o700)
		} else {
			_ = os.Chmod(path, 0o700)
		}
		return nil
	})
}

func TestTaggedFilename(t *testing.T) {
	cases := [][3]string{
		{filepath.Join("foo", "bar.baz"), "tag", filepath.Join("foo", "bar~tag.baz")},
		{"bar.baz", "tag", "bar~tag.baz"},
		{"bar", "tag", "bar~tag"},
		{"~$ufheft2.docx", "20140612-200554", "~$ufheft2~20140612-200554.docx"},
		{"alle~4.mgz", "20141106-094415", "alle~4~20141106-094415.mgz"},

		// Parsing test only
		{"", "tag-only", "foo/bar.baz~tag-only"},
		{"", "tag-only", "bar.baz~tag-only"},
		{"", "20140612-200554", "~$ufheft2.docx~20140612-200554"},
		{"", "20141106-094415", "alle~4.mgz~20141106-094415"},
	}

	for _, tc := range cases {
		if tc[0] != "" {
			// Test tagger
			tf := TagFilename(tc[0], tc[1])
			if tf != tc[2] {
				t.Errorf("%s != %s", tf, tc[2])
			}
		}

		// Test parser
		tag := extractTag(tc[2])
		if tag != tc[1] {
			t.Errorf("%s != %s", tag, tc[1])
		}
	}
}

func TestSimpleVersioningVersionCount(t *testing.T) {
	if testing.Short() {
		t.Skip("Test takes some time, skipping.")
	}

	dir := t.TempDir()

	cfg := config.FolderConfiguration{
		FilesystemType: config.FilesystemTypeBasic,
		Path:           dir,
		Versioning: config.VersioningConfiguration{
			Params: map[string]string{
				"keep": "2",
			},
		},
	}
	fs := cfg.Filesystem()

	v := newSimple(cfg)

	path := "test"

	for i := 1; i <= 3; i++ {
		f, err := fs.Create(path)
		if err != nil {
			t.Error(err)
		}
		f.Close()
		if err := v.Archive(path); err != nil {
			t.Error(err)
		}

		n, err := fs.DirNames(DefaultPath)
		if err != nil {
			t.Error(err)
		}

		if len(n) != min(i, 2) {
			t.Error("Wrong count")
		}

		time.Sleep(time.Second)
	}
}

func TestPathTildes(t *testing.T) {
	// Test that folder and version paths with leading tildes are expanded
	// to the user's home directory. (issue #9241)
	home := t.TempDir()
	t.Setenv("HOME", home)
	if vn := filepath.VolumeName(home); vn != "" {
		// Legacy Windows home stuff
		t.Setenv("HomeDrive", vn)
		t.Setenv("HomePath", strings.TrimPrefix(home, vn))
	}
	os.Mkdir(filepath.Join(home, "folder"), 0o755)

	cfg := config.FolderConfiguration{
		FilesystemType: config.FilesystemTypeBasic,
		Path:           "~/folder",
		Versioning: config.VersioningConfiguration{
			FSPath: "~/versions",
			FSType: config.FilesystemTypeBasic,
			Params: map[string]string{
				"keep": "2",
			},
		},
	}
	fs := cfg.Filesystem()
	v := newSimple(cfg)

	const testPath = "test"

	f, err := fs.Create(testPath)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := v.Archive(testPath); err != nil {
		t.Fatal(err)
	}

	// Check that there are no entries in the folder directory; this is
	// specifically to check that there is no directory named "~" there.
	names, err := fs.DirNames(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Fatalf("found %d files in folder dir, want 0", len(names))
	}

	// Check that the versions directory contains one file that begins with
	// our test path.
	des, err := os.ReadDir(filepath.Join(home, "versions"))
	if err != nil {
		t.Fatal(err)
	}
	for _, de := range des {
		names = append(names, de.Name())
	}
	if len(names) != 1 {
		t.Fatalf("found %d files in versions dir, want 1", len(names))
	}
	if got := names[0]; !strings.HasPrefix(got, testPath) {
		t.Fatalf("found versioned file %q, want one that begins with %q", got, testPath)
	}
}

func TestArchiveFoldersCreationPermission(t *testing.T) {
	if build.IsWindows {
		t.Skip("Skipping on Windows")
		return
	}
	dir := t.TempDir()
	versionsDir := t.TempDir()
	t.Cleanup(func() {
		makeTreeOwnerWritable(t, versionsDir)
		makeTreeOwnerWritable(t, dir)
	})

	cfg := config.FolderConfiguration{
		FilesystemType: config.FilesystemTypeBasic,
		Path:           dir,
		Versioning: config.VersioningConfiguration{
			FSPath: versionsDir,
			FSType: config.FilesystemTypeBasic,
			Params: map[string]string{
				"keep": "2",
			},
		},
	}
	vfs := cfg.Filesystem()
	v := newSimple(cfg)

	// Create two folders and set their permissions
	folder1Path := filepath.Join(dir, "folder1")
	folder1Perms := os.FileMode(0o777)
	folder1VersionsPath := filepath.Join(versionsDir, "folder1")
	err := os.Mkdir(folder1Path, folder1Perms)
	if err != nil {
		t.Fatal(err)
	}
	// chmod incase umask changes the create permissions
	err = os.Chmod(folder1Path, folder1Perms)
	if err != nil {
		t.Fatal(err)
	}

	folder2Path := filepath.Join(folder1Path, "földer2")
	folder2VersionsPath := filepath.Join(folder1VersionsPath, "földer2")
	folder2Perms := os.FileMode(0o744)
	err = os.Mkdir(folder2Path, folder2Perms)
	if err != nil {
		t.Fatal(err)
	}
	// chmod incase umask changes the create permissions
	err = os.Chmod(folder2Path, folder2Perms)
	if err != nil {
		t.Fatal(err)
	}

	// create a file
	filePath := filepath.Join("folder1", "földer2", "testFile")
	f, err := vfs.Create(filePath)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	if err := v.Archive(filePath); err != nil {
		t.Error(err)
	}

	// check permissions of the created version folders
	folder1VersionsInfo, err := os.Stat(folder1VersionsPath)
	if err != nil {
		t.Fatal(err)
	}
	if folder1VersionsInfo.Mode().Perm() != folder1Perms {
		t.Errorf("folder1 permissions %v, want %v", folder1VersionsInfo.Mode(), folder1Perms)
	}

	folder2VersionsInfo, err := os.Stat(folder2VersionsPath)
	if err != nil {
		t.Fatal(err)
	}
	if folder2VersionsInfo.Mode().Perm() != folder2Perms {
		t.Errorf("földer2 permissions %v, want %v", folder2VersionsInfo.Mode(), folder2Perms)
	}

	// Archive again to test that archiving doesn't fail if the versioned folders already exist
	if err := v.Archive(filePath); err != nil {
		t.Error(err)
	}
	folder1VersionsInfo, err = os.Stat(folder1VersionsPath)
	if err != nil {
		t.Fatal(err)
	}
	if folder1VersionsInfo.Mode().Perm() != folder1Perms {
		t.Errorf("folder1 permissions %v, want %v", folder1VersionsInfo.Mode(), folder1Perms)
	}

	folder2VersionsInfo, err = os.Stat(folder2VersionsPath)
	if err != nil {
		t.Fatal(err)
	}
	if folder2VersionsInfo.Mode().Perm() != folder2Perms {
		t.Errorf("földer2 permissions %v, want %v", folder2VersionsInfo.Mode(), folder2Perms)
	}
}

func TestArchiveReadOnlyVersionsParent(t *testing.T) {
	if build.IsWindows {
		t.Skip("Skipping on Windows")
		return
	}

	dir := t.TempDir()
	versionsDir := t.TempDir()
	t.Cleanup(func() {
		makeTreeOwnerWritable(t, versionsDir)
		makeTreeOwnerWritable(t, dir)
	})

	cfg := config.FolderConfiguration{
		FilesystemType: config.FilesystemTypeBasic,
		Path:           dir,
		Versioning: config.VersioningConfiguration{
			FSPath: versionsDir,
			FSType: config.FilesystemTypeBasic,
			Params: map[string]string{
				"keep": "2",
			},
		},
	}

	vfs := cfg.Filesystem()
	v := newSimple(cfg)

	if err := os.MkdirAll(filepath.Join(dir, "a", "b", "c"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(versionsDir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(versionsDir, "a", "b"), 0o555); err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join("a", "b", "c", "testFile")
	f, err := vfs.Create(filePath)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	if err := v.Archive(filePath); err != nil {
		t.Fatalf("archiving failed with read-only versions parent dir: %v", err)
	}

	if _, err := os.Stat(filepath.Join(versionsDir, "a", "b", "c")); err != nil {
		t.Fatalf("expected nested versions directory to exist: %v", err)
	}

	if err := os.Chmod(filepath.Join(versionsDir, "a", "b", "c"), 0o555); err != nil {
		t.Fatal(err)
	}

	f, err = vfs.Create(filePath)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	if err := v.Archive(filePath); err != nil {
		t.Fatalf("archiving failed with read-only versions target dir: %v", err)
	}

	info, err := os.Stat(filepath.Join(versionsDir, "a", "b"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o555 {
		t.Fatalf("expected versions parent dir permissions restored to 0555, got %v", info.Mode())
	}

	info, err = os.Stat(filepath.Join(versionsDir, "a", "b", "c"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o555 {
		t.Fatalf("expected versions target dir permissions restored to 0555, got %v", info.Mode())
	}
}

func TestArchiveReadOnlySourceIntermediateDir(t *testing.T) {
	if build.IsWindows {
		t.Skip("Skipping on Windows")
		return
	}

	dir := t.TempDir()
	versionsDir := t.TempDir()
	t.Cleanup(func() {
		makeTreeOwnerWritable(t, versionsDir)
		makeTreeOwnerWritable(t, dir)
	})

	cfg := config.FolderConfiguration{
		FilesystemType: config.FilesystemTypeBasic,
		Path:           dir,
		Versioning: config.VersioningConfiguration{
			FSPath: versionsDir,
			FSType: config.FilesystemTypeBasic,
			Params: map[string]string{
				"keep": "2",
			},
		},
	}

	vfs := cfg.Filesystem()
	v := newSimple(cfg)

	if err := os.MkdirAll(filepath.Join(dir, "a", "b", "c"), 0o755); err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join("a", "b", "c", "testFile")
	f, err := vfs.Create(filePath)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	if err := os.Chmod(filepath.Join(dir, "a", "b"), 0o555); err != nil {
		t.Fatal(err)
	}

	if err := v.Archive(filePath); err != nil {
		t.Fatalf("archiving failed with read-only source intermediate dir: %v", err)
	}

	if _, err := os.Stat(filepath.Join(versionsDir, "a", "b", "c")); err != nil {
		t.Fatalf("expected nested versions directory to exist: %v", err)
	}

	info, err := os.Stat(filepath.Join(versionsDir, "a", "b"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o555 {
		t.Fatalf("expected versions intermediate dir permissions preserved as 0555, got %v", info.Mode())
	}
}

// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
)

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

func TestArchiveUnderRestrictiveVersionDir(t *testing.T) {
	// Test that an archive below a .stversions directory left at 0o555 by
	// prior source-perm mirroring still succeeds, with the directory mode
	// restored afterwards. (issue #10532)
	if build.IsWindows {
		t.Skip("Skipping on Windows")
		return
	}
	if os.Geteuid() == 0 {
		t.Skip("Skipping as root: DAC permission checks are bypassed")
		return
	}
	dir := t.TempDir()
	versionsDir := t.TempDir()

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

	// Source tree: folder/sub/file. Sub gets a non-default mode so we
	// can verify the mirroring continues to work for new dirs.
	folder := filepath.Join(dir, "folder")
	sub := filepath.Join(folder, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join("folder", "sub", "file")
	f, err := vfs.Create(rel)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	subPerms := os.FileMode(0o750)
	if err := os.Chmod(sub, subPerms); err != nil {
		t.Fatal(err)
	}

	// Pre-populate .stversions/folder at 0o555 to mimic the pathological
	// state where prior mirroring left an intermediate version directory
	// without owner-write.
	versionedFolder := filepath.Join(versionsDir, "folder")
	if err := os.Mkdir(versionedFolder, 0o755); err != nil {
		t.Fatal(err)
	}
	folderPerms := os.FileMode(0o555)
	if err := os.Chmod(versionedFolder, folderPerms); err != nil {
		t.Fatal(err)
	}
	// Restore owner-write so t.TempDir cleanup can remove the tree.
	t.Cleanup(func() { _ = os.Chmod(versionedFolder, 0o755) })

	if err := v.Archive(rel); err != nil {
		t.Fatalf("archive: %v", err)
	}

	// The pre-existing folder dir must be back at 0o555.
	info, err := os.Stat(versionedFolder)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != folderPerms {
		t.Errorf("folder restored mode %v, want %v", info.Mode().Perm(), folderPerms)
	}

	// The newly created sub dir under it must mirror the source mode.
	versionedSub := filepath.Join(versionedFolder, "sub")
	info, err = os.Stat(versionedSub)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != subPerms {
		t.Errorf("sub mode %v, want %v", info.Mode().Perm(), subPerms)
	}

	// And the file must have landed inside it.
	entries, err := os.ReadDir(versionedSub)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("found %d entries in sub version dir, want 1", len(entries))
	}

	// Archive another file under the same path to exercise the
	// chmod-restore cycle a second time, with both folder and sub now
	// pre-existing at their restored modes.
	rel2 := filepath.Join("folder", "sub", "file2")
	f2, err := vfs.Create(rel2)
	if err != nil {
		t.Fatal(err)
	}
	f2.Close()
	if err := v.Archive(rel2); err != nil {
		t.Fatalf("second archive: %v", err)
	}
	info, err = os.Stat(versionedFolder)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != folderPerms {
		t.Errorf("folder mode after second archive %v, want %v", info.Mode().Perm(), folderPerms)
	}
}

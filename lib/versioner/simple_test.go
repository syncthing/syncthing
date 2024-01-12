// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"

	"github.com/syncthing/syncthing/lib/fs"
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
		FilesystemType: fs.FilesystemTypeBasic,
		Path:           dir,
		Versioning: config.VersioningConfiguration{
			Params: map[string]string{
				"keep": "2",
			},
		},
	}
	fs := cfg.Filesystem(nil)

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

		if float64(len(n)) != math.Min(float64(i), 2) {
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
		FilesystemType: fs.FilesystemTypeBasic,
		Path:           "~/folder",
		Versioning: config.VersioningConfiguration{
			FSPath: "~/versions",
			FSType: fs.FilesystemTypeBasic,
			Params: map[string]string{
				"keep": "2",
			},
		},
	}
	fs := cfg.Filesystem(nil)
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

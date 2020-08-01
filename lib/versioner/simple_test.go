// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"context"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
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

func TestSimpleCleanOut(t *testing.T) {
	var testcases = []struct {
		file         string
		shouldRemove bool
	}{
		{"testdata/.stversions/file1", false},
		{"testdata/.stversions/file2", true},
		{"testdata/.stversions/keep1/file1", false},
		{"testdata/.stversions/keep1/file2", false},
		{"testdata/.stversions/keep2/file1", false},
		{"testdata/.stversions/keep2/file2", true},
		{"testdata/.stversions/keep3/keepsubdir/file1", false},
		{"testdata/.stversions/remove/file1", true},
		{"testdata/.stversions/remove/file2", true},
		{"testdata/.stversions/remove/removesubdir/file1", true},
	}

	os.RemoveAll("testdata")
	defer os.RemoveAll("testdata")

	oldTime := time.Now().Add(-8 * 24 * time.Hour)
	for _, tc := range testcases {
		os.MkdirAll(filepath.Dir(tc.file), 0777)
		if err := ioutil.WriteFile(tc.file, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
		if tc.shouldRemove {
			if err := os.Chtimes(tc.file, oldTime, oldTime); err != nil {
				t.Fatal(err)
			}
		}
	}

	cfg := config.FolderConfiguration{
		FilesystemType: fs.FilesystemTypeBasic,
		Path:           "testdata",
		Versioning: config.VersioningConfiguration{
			Params: map[string]string{
				"cleanoutDays": "7",
			},
		},
	}

	versioner := newSimple(cfg)
	if err := versioner.Clean(context.Background()); err != nil {
		t.Fatal(err)
	}

	for _, tc := range testcases {
		_, err := os.Lstat(tc.file)
		if tc.shouldRemove && !os.IsNotExist(err) {
			t.Error(tc.file, "should have been removed")
		} else if !tc.shouldRemove && err != nil {
			t.Error(tc.file, "should not have been removed")
		}
	}

	if _, err := os.Lstat("testdata/.stversions/keep3"); os.IsNotExist(err) {
		t.Error("directory with non empty subdirs should not be removed")
	}

	if _, err := os.Lstat("testdata/.stversions/remove"); !os.IsNotExist(err) {
		t.Error("empty directory should have been removed")
	}
}

func TestSimpleVersioningVersionCount(t *testing.T) {
	if testing.Short() {
		t.Skip("Test takes some time, skipping.")
	}

	dir, err := ioutil.TempDir("", "")
	//defer os.RemoveAll(dir)
	if err != nil {
		t.Error(err)
	}

	cfg := config.FolderConfiguration{
		FilesystemType: fs.FilesystemTypeBasic,
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

		n, err := fs.DirNames(".stversions")
		if err != nil {
			t.Error(err)
		}

		if float64(len(n)) != math.Min(float64(i), 2) {
			t.Error("Wrong count")
		}

		time.Sleep(time.Second)
	}
}

// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package versioner

import (
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
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
			tf := taggedFilename(tc[0], tc[1])
			if tf != tc[2] {
				t.Errorf("%s != %s", tf, tc[2])
			}
		}

		// Test parser
		tag := filenameTag(tc[2])
		if tag != tc[1] {
			t.Errorf("%s != %s", tag, tc[1])
		}
	}
}

func TestSimpleVersioningVersionCount(t *testing.T) {
	if testing.Short() {
		t.Skip("Test takes some time, skipping.")
	}

	dir, err := ioutil.TempDir("", "")
	defer os.RemoveAll(dir)
	if err != nil {
		t.Error(err)
	}

	v := NewSimple("", dir, map[string]string{"keep": "2"})
	versionDir := filepath.Join(dir, ".stversions")

	path := filepath.Join(dir, "test")

	for i := 1; i <= 3; i++ {
		f, err := os.Create(path)
		if err != nil {
			t.Error(err)
		}
		f.Close()
		v.Archive(path)

		d, err := os.Open(versionDir)
		if err != nil {
			t.Error(err)
		}
		n, err := d.Readdirnames(-1)
		if err != nil {
			t.Error(err)
		}

		if float64(len(n)) != math.Min(float64(i), 2) {
			t.Error("Wrong count")
		}
		d.Close()

		time.Sleep(time.Second)
	}
}

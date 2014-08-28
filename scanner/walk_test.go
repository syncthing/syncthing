// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package scanner

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/syncthing/syncthing/protocol"
)

var testdata = []struct {
	name string
	size int
	hash string
}{
	{"bar", 10, "2f72cc11a6fcd0271ecef8c61056ee1eb1243be3805bf9a9df98f92f7636b05c"},
	{"baz", 0, ""},
	{"empty", 0, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
	{"foo", 7, "aec070645fe53ee3b3763059376134f058cc337247c978add178b6ccdfb0019f"},
}

var correctIgnores = map[string][]string{
	".": {".*", "quux"},
}

func TestWalkSub(t *testing.T) {
	w := Walker{
		Dir:        "testdata",
		Sub:        "foo",
		BlockSize:  128 * 1024,
		IgnoreFile: ".stignore",
	}
	fchan, err := w.Walk()
	var files []protocol.FileInfo
	for f := range fchan {
		files = append(files, f)
	}
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 1 {
		t.Fatalf("Incorrect length %d != 1", len(files))
	}
	if files[0].Name != "foo" {
		t.Errorf("Incorrect file %v != foo", files[0])
	}
}

func TestWalk(t *testing.T) {
	w := Walker{
		Dir:        "testdata",
		BlockSize:  128 * 1024,
		IgnoreFile: ".stignore",
	}
	fchan, err := w.Walk()
	var files []protocol.FileInfo
	for f := range fchan {
		files = append(files, f)
	}
	sort.Sort(fileList(files))

	if err != nil {
		t.Fatal(err)
	}

	if l1, l2 := len(files), len(testdata); l1 != l2 {
		t.Log(files)
		t.Log(testdata)
		t.Fatalf("Incorrect number of walked files %d != %d", l1, l2)
	}

	for i := range testdata {
		if n1, n2 := testdata[i].name, files[i].Name; n1 != n2 {
			t.Errorf("Incorrect file name %q != %q for case #%d", n1, n2, i)
		}

		if testdata[i].hash != "" {
			if h1, h2 := fmt.Sprintf("%x", files[i].Blocks[0].Hash), testdata[i].hash; h1 != h2 {
				t.Errorf("Incorrect hash %q != %q for case #%d", h1, h2, i)
			}
		}

		t0 := time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
		t1 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
		if mt := files[i].Modified; mt < t0 || mt > t1 {
			t.Errorf("Unrealistic modtime %d for test %d", mt, i)
		}
	}
}

func TestWalkError(t *testing.T) {
	w := Walker{
		Dir:        "testdata-missing",
		BlockSize:  128 * 1024,
		IgnoreFile: ".stignore",
	}
	_, err := w.Walk()

	if err == nil {
		t.Error("no error from missing directory")
	}

	w = Walker{
		Dir:        "testdata/bar",
		BlockSize:  128 * 1024,
		IgnoreFile: ".stignore",
	}
	_, err = w.Walk()

	if err == nil {
		t.Error("no error from non-directory")
	}
}

func TestIgnore(t *testing.T) {
	patStr := bytes.NewBufferString(`
		t2
		/t3
		sub/dir/*
		*/other/test
		**/deep
	`)
	patterns := parseIgnoreFile(patStr, "", "")

	patStr = bytes.NewBufferString(`
		bar
		z*
		q[abc]x
	`)
	patterns = append(patterns, parseIgnoreFile(patStr, "foo", "")...)

	patStr = bytes.NewBufferString(`
		quux
		.*
	`)
	patterns = append(patterns, parseIgnoreFile(patStr, "foo/baz", "")...)

	var tests = []struct {
		f string
		r bool
	}{
		{filepath.Join("foo", "bar"), true},
		{filepath.Join("t3"), true},
		{filepath.Join("foofoo"), false},
		{filepath.Join("foo", "quux"), false},
		{filepath.Join("foo", "zuux"), true},
		{filepath.Join("foo", "qzuux"), false},
		{filepath.Join("foo", "baz", "t1"), false},
		{filepath.Join("foo", "baz", "t2"), true},
		{filepath.Join("foo", "baz", "t3"), false},
		{filepath.Join("foo", "baz", "bar"), true},
		{filepath.Join("foo", "baz", "quuxa"), false},
		{filepath.Join("foo", "baz", "aquux"), false},
		{filepath.Join("foo", "baz", ".quux"), true},
		{filepath.Join("foo", "baz", "zquux"), true},
		{filepath.Join("foo", "baz", "quux"), true},
		{filepath.Join("foo", "bazz", "quux"), false},
		{filepath.Join("sub", "dir", "hej"), true},
		{filepath.Join("deeper", "sub", "dir", "hej"), true},
		{filepath.Join("other", "test"), false},
		{filepath.Join("sub", "other", "test"), true},
		{filepath.Join("deeper", "sub", "other", "test"), true},
		{filepath.Join("deep"), true},
		{filepath.Join("deeper", "deep"), true},
		{filepath.Join("deeper", "deeper", "deep"), true},
	}

	w := Walker{}
	for i, tc := range tests {
		if r := w.ignoreFile(patterns, tc.f); r != tc.r {
			t.Errorf("Incorrect ignoreFile() #%d (%s); E: %v, A: %v", i, tc.f, tc.r, r)
		}
	}
}

type fileList []protocol.FileInfo

func (f fileList) Len() int {
	return len(f)
}

func (f fileList) Less(a, b int) bool {
	return f[a].Name < f[b].Name
}

func (f fileList) Swap(a, b int) {
	f[a], f[b] = f[b], f[a]
}

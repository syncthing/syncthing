// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package scanner

import (
	"fmt"
	"reflect"
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

func TestWalk(t *testing.T) {
	w := Walker{
		Dir:        "testdata",
		BlockSize:  128 * 1024,
		IgnoreFile: ".stignore",
	}
	fchan, ignores, err := w.Walk()
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

	if !reflect.DeepEqual(ignores, correctIgnores) {
		t.Errorf("Incorrect ignores\n  %v\n  %v", correctIgnores, ignores)
	}
}

func TestWalkError(t *testing.T) {
	w := Walker{
		Dir:        "testdata-missing",
		BlockSize:  128 * 1024,
		IgnoreFile: ".stignore",
	}
	_, _, err := w.Walk()

	if err == nil {
		t.Error("no error from missing directory")
	}

	w = Walker{
		Dir:        "testdata/bar",
		BlockSize:  128 * 1024,
		IgnoreFile: ".stignore",
	}
	_, _, err = w.Walk()

	if err == nil {
		t.Error("no error from non-directory")
	}
}

func TestIgnore(t *testing.T) {
	var patterns = map[string][]string{
		".":       {"t2"},
		"foo":     {"bar", "z*", "q[abc]x", "q\\[abc\\]y"},
		"foo/baz": {"quux", ".*"},
	}
	var tests = []struct {
		f string
		r bool
	}{
		{"foo/bar", true},
		{"foofoo", false},
		{"foo/quux", false},
		{"foo/zuux", true},
		{"foo/qzuux", false},
		{"foo/baz/t1", false},
		{"foo/baz/t2", true},
		{"foo/baz/bar", true},
		{"foo/baz/quuxa", false},
		{"foo/baz/aquux", false},
		{"foo/baz/.quux", true},
		{"foo/baz/zquux", true},
		{"foo/baz/quux", true},
		{"foo/bazz/quux", false},
		{"foo/bazz/q[abc]x", false},
		{"foo/bazz/qax", true},
		{"foo/bazz/q[abc]y", true},
	}

	w := Walker{}
	for i, tc := range tests {
		if r := w.ignoreFile(patterns, tc.f); r != tc.r {
			t.Errorf("Incorrect ignoreFile() #%d; E: %v, A: %v", i, tc.r, r)
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

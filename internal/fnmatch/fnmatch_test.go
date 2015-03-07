// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package fnmatch

import (
	"path/filepath"
	"runtime"
	"testing"
)

type testcase struct {
	pat   string
	name  string
	flags int
	match bool
}

var testcases = []testcase{
	{"", "", 0, true},
	{"*", "", 0, true},
	{"*", "foo", 0, true},
	{"*", "bar", 0, true},
	{"*", "*", 0, true},
	{"**", "f", 0, true},
	{"**", "foo.txt", 0, true},
	{"*.*", "foo.txt", 0, true},
	{"foo*.txt", "foobar.txt", 0, true},
	{"foo.txt", "foo.txt", 0, true},

	{"foo.txt", "bar/foo.txt", 0, false},
	{"*/foo.txt", "bar/foo.txt", 0, true},
	{"f?o.txt", "foo.txt", 0, true},
	{"f?o.txt", "fooo.txt", 0, false},
	{"f[ab]o.txt", "foo.txt", 0, false},
	{"f[ab]o.txt", "fao.txt", 0, true},
	{"f[ab]o.txt", "fbo.txt", 0, true},
	{"f[ab]o.txt", "fco.txt", 0, false},
	{"f[ab]o.txt", "fabo.txt", 0, false},
	{"f[ab]o.txt", "f[ab]o.txt", 0, false},
	{"f\\[ab\\]o.txt", "f[ab]o.txt", FNM_NOESCAPE, false},

	{"*foo.txt", "bar/foo.txt", 0, true},
	{"*foo.txt", "bar/foo.txt", FNM_PATHNAME, false},
	{"*/foo.txt", "bar/foo.txt", 0, true},
	{"*/foo.txt", "bar/foo.txt", FNM_PATHNAME, true},
	{"*/foo.txt", "bar/baz/foo.txt", 0, true},
	{"*/foo.txt", "bar/baz/foo.txt", FNM_PATHNAME, false},
	{"**/foo.txt", "bar/baz/foo.txt", 0, true},
	{"**/foo.txt", "bar/baz/foo.txt", FNM_PATHNAME, true},

	{"foo.txt", "foo.TXT", FNM_CASEFOLD, true},

	// These characters are literals in glob, but not in regexp.
	{"hey$hello", "hey$hello", 0, true},
	{"hey^hello", "hey^hello", 0, true},
	{"hey{hello", "hey{hello", 0, true},
	{"hey}hello", "hey}hello", 0, true},
	{"hey(hello", "hey(hello", 0, true},
	{"hey)hello", "hey)hello", 0, true},
	{"hey|hello", "hey|hello", 0, true},
	{"hey|hello", "hey|other", 0, false},
}

func TestMatch(t *testing.T) {
	switch runtime.GOOS {
	case "windows":
		testcases = append(testcases, testcase{"foo.txt", "foo.TXT", 0, true})
	case "darwin":
		testcases = append(testcases, testcase{"foo.txt", "foo.TXT", 0, true})
		fallthrough
	default:
		testcases = append(testcases, testcase{"f\\[ab\\]o.txt", "f[ab]o.txt", 0, true})
		testcases = append(testcases, testcase{"foo\\.txt", "foo.txt", 0, true})
		testcases = append(testcases, testcase{"foo\\*.txt", "foo*.txt", 0, true})
		testcases = append(testcases, testcase{"foo\\.txt", "foo.txt", FNM_NOESCAPE, false})
		testcases = append(testcases, testcase{"f\\\\\\[ab\\\\\\]o.txt", "f\\[ab\\]o.txt", 0, true})
	}

	for _, tc := range testcases {
		if m, err := Match(tc.pat, filepath.FromSlash(tc.name), tc.flags); m != tc.match {
			if err != nil {
				t.Error(err)
			} else {
				t.Errorf("Match(%q, %q, %d) != %v", tc.pat, tc.name, tc.flags, tc.match)
			}
		}
	}
}

func TestInvalid(t *testing.T) {
	if _, err := Match("foo[bar", "...", 0); err == nil {
		t.Error("Unexpected nil error")
	}
}

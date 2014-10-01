// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

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

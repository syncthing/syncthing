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

package ignore_test

import (
	"bytes"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/syncthing/syncthing/internal/ignore"
)

func TestIgnore(t *testing.T) {
	pats, err := ignore.Load("testdata/.stignore")
	if err != nil {
		t.Fatal(err)
	}

	var tests = []struct {
		f string
		r bool
	}{
		{"afile", false},
		{"bfile", true},
		{"cfile", false},
		{"dfile", false},
		{"efile", true},
		{"ffile", true},

		{"dir1", false},
		{filepath.Join("dir1", "cfile"), true},
		{filepath.Join("dir1", "dfile"), false},
		{filepath.Join("dir1", "efile"), true},
		{filepath.Join("dir1", "ffile"), false},

		{"dir2", false},
		{filepath.Join("dir2", "cfile"), false},
		{filepath.Join("dir2", "dfile"), true},
		{filepath.Join("dir2", "efile"), true},
		{filepath.Join("dir2", "ffile"), false},

		{filepath.Join("dir3"), true},
		{filepath.Join("dir3", "afile"), true},
	}

	for i, tc := range tests {
		if r := pats.Match(tc.f); r != tc.r {
			t.Errorf("Incorrect ignoreFile() #%d (%s); E: %v, A: %v", i, tc.f, tc.r, r)
		}
	}
}

func TestExcludes(t *testing.T) {
	stignore := `
	!iex2
	!ign1/ex
	ign1
	i*2
	!ign2
	`
	pats, err := ignore.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		t.Fatal(err)
	}

	var tests = []struct {
		f string
		r bool
	}{
		{"ign1", true},
		{"ign2", true},
		{"ibla2", true},
		{"iex2", false},
		{filepath.Join("ign1", "ign"), true},
		{filepath.Join("ign1", "ex"), false},
		{filepath.Join("ign1", "iex2"), false},
		{filepath.Join("iex2", "ign"), false},
		{filepath.Join("foo", "bar", "ign1"), true},
		{filepath.Join("foo", "bar", "ign2"), true},
		{filepath.Join("foo", "bar", "iex2"), false},
	}

	for _, tc := range tests {
		if r := pats.Match(tc.f); r != tc.r {
			t.Errorf("Incorrect match for %s: %v != %v", tc.f, r, tc.r)
		}
	}
}

func TestBadPatterns(t *testing.T) {
	var badPatterns = []string{
		"[",
		"/[",
		"**/[",
		"#include nonexistent",
		"#include .stignore",
		"!#include makesnosense",
	}

	for _, pat := range badPatterns {
		parsed, err := ignore.Parse(bytes.NewBufferString(pat), ".stignore")
		if err == nil {
			t.Errorf("No error for pattern %q: %v", pat, parsed)
		}
	}
}

func TestCaseSensitivity(t *testing.T) {
	ign, _ := ignore.Parse(bytes.NewBufferString("test"), ".stignore")

	match := []string{"test"}
	dontMatch := []string{"foo"}

	switch runtime.GOOS {
	case "darwin", "windows":
		match = append(match, "TEST", "Test", "tESt")
	default:
		dontMatch = append(dontMatch, "TEST", "Test", "tESt")
	}

	for _, tc := range match {
		if !ign.Match(tc) {
			t.Errorf("Incorrect match for %q: should be matched", tc)
		}
	}

	for _, tc := range dontMatch {
		if ign.Match(tc) {
			t.Errorf("Incorrect match for %q: should not be matched", tc)
		}
	}
}

func TestCommentsAndBlankLines(t *testing.T) {
	stignore := `
	// foo
	//bar

	//!baz
	//#dex

	//                        ips


	`
	pats, _ := ignore.Parse(bytes.NewBufferString(stignore), ".stignore")
	if len(pats) > 0 {
		t.Errorf("Expected no patterns")
	}
}

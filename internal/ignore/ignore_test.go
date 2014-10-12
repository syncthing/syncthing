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

package ignore

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestIgnore(t *testing.T) {
	pats, err := Load("testdata/.stignore", true)
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
	pats, err := Parse(bytes.NewBufferString(stignore), ".stignore")
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
		parsed, err := Parse(bytes.NewBufferString(pat), ".stignore")
		if err == nil {
			t.Errorf("No error for pattern %q: %v", pat, parsed)
		}
	}
}

func TestCaseSensitivity(t *testing.T) {
	ign, _ := Parse(bytes.NewBufferString("test"), ".stignore")

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

func TestCaching(t *testing.T) {
	fd1, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatal(err)
	}

	fd2, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatal(err)
	}

	defer fd1.Close()
	defer fd2.Close()
	defer os.Remove(fd1.Name())
	defer os.Remove(fd2.Name())

	_, err = fd1.WriteString("/x/\n#include " + filepath.Base(fd2.Name()) + "\n")
	if err != nil {
		t.Fatal(err)
	}

	fd2.WriteString("/y/\n")

	pats, err := Load(fd1.Name(), true)
	if err != nil {
		t.Fatal(err)
	}

	if pats.oldMatches == nil || len(pats.oldMatches) != 0 {
		t.Fatal("Expected empty map")
	}

	if pats.newMatches == nil || len(pats.newMatches) != 0 {
		t.Fatal("Expected empty map")
	}

	if len(pats.patterns) != 4 {
		t.Fatal("Incorrect number of patterns loaded", len(pats.patterns), "!=", 4)
	}

	// Cache some outcomes

	for _, letter := range []string{"a", "b", "x", "y"} {
		pats.Match(letter)
	}

	if len(pats.newMatches) != 4 {
		t.Fatal("Expected 4 cached results")
	}

	// Reload file, expect old outcomes to be provided

	pats, err = Load(fd1.Name(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(pats.oldMatches) != 4 {
		t.Fatal("Expected 4 cached results")
	}

	// Match less this time

	for _, letter := range []string{"b", "x", "y"} {
		pats.Match(letter)
	}

	if len(pats.newMatches) != 3 {
		t.Fatal("Expected 3 cached results")
	}

	// Reload file, expect the new outcomes to be provided

	pats, err = Load(fd1.Name(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(pats.oldMatches) != 3 {
		t.Fatal("Expected 3 cached results", len(pats.oldMatches))
	}

	// Modify the include file, expect empty cache

	fd2.WriteString("/z/\n")

	pats, err = Load(fd1.Name(), true)
	if err != nil {
		t.Fatal(err)
	}

	if len(pats.oldMatches) != 0 {
		t.Fatal("Expected 0 cached results")
	}

	// Cache some outcomes again

	for _, letter := range []string{"b", "x", "y"} {
		pats.Match(letter)
	}

	// Verify that outcomes provided on next laod

	pats, err = Load(fd1.Name(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(pats.oldMatches) != 3 {
		t.Fatal("Expected 3 cached results")
	}

	// Modify the root file, expect cache to be invalidated

	fd1.WriteString("/a/\n")

	pats, err = Load(fd1.Name(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(pats.oldMatches) != 0 {
		t.Fatal("Expected cache invalidation")
	}

	// Cache some outcomes again

	for _, letter := range []string{"b", "x", "y"} {
		pats.Match(letter)
	}

	// Verify that outcomes provided on next laod

	pats, err = Load(fd1.Name(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(pats.oldMatches) != 3 {
		t.Fatal("Expected 3 cached results")
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
	pats, _ := Parse(bytes.NewBufferString(stignore), ".stignore")
	if len(pats.patterns) > 0 {
		t.Errorf("Expected no patterns")
	}
}

var result bool

func BenchmarkMatch(b *testing.B) {
	stignore := `
.frog
.frog*
.frogfox
.whale
.whale/*
.dolphin
.dolphin/*
~ferret~.*
.ferret.*
flamingo.*
flamingo
*.crow
*.crow
	`
	pats, _ := Parse(bytes.NewBufferString(stignore), ".stignore")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result = pats.Match("filename")
	}
}

func BenchmarkMatchCached(b *testing.B) {
	stignore := `
.frog
.frog*
.frogfox
.whale
.whale/*
.dolphin
.dolphin/*
~ferret~.*
.ferret.*
flamingo.*
flamingo
*.crow
*.crow
	`
	// Caches per file, hence write the patterns to a file.
	fd, err := ioutil.TempFile("", "")
	if err != nil {
		b.Fatal(err)
	}

	_, err = fd.WriteString(stignore)
	defer fd.Close()
	defer os.Remove(fd.Name())
	if err != nil {
		b.Fatal(err)
	}

	// Load the patterns
	pats, err := Load(fd.Name(), true)
	if err != nil {
		b.Fatal(err)
	}
	// Cache the outcome for "filename"
	pats.Match("filename")

	// This load should now load the cached outcomes as the set of patterns
	// has not changed.
	pats, err = Load(fd.Name(), true)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result = pats.Match("filename")
	}
}

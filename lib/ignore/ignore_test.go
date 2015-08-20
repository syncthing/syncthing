// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

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
	pats := New(true)
	err := pats.Load("testdata/.stignore", "testdata")
	if err != nil {
		t.Fatal(err)
	}

	var tests = []struct {
		f string
		r Result
	}{
		{"afile", DontIgnore},
		{"bfile", Nuke},
		{"cfile", DontIgnore},
		{"dfile", DontIgnore},
		{"efile", Nuke},
		{"ffile", Nuke},

		{"dir1", DontIgnore},
		{filepath.Join("dir1", "cfile"), Nuke},
		{filepath.Join("dir1", "dfile"), DontIgnore},
		{filepath.Join("dir1", "efile"), Nuke},
		{filepath.Join("dir1", "ffile"), DontIgnore},

		{"dir2", DontIgnore},
		{filepath.Join("dir2", "cfile"), DontIgnore},
		{filepath.Join("dir2", "dfile"), Nuke},
		{filepath.Join("dir2", "efile"), Nuke},
		{filepath.Join("dir2", "ffile"), DontIgnore},

		{filepath.Join("dir3"), Nuke},
		{filepath.Join("dir3", "afile"), Nuke},

		{"lost+found", Nuke},
	}

	for i, tc := range tests {
		if r := pats.match(tc.f); r != tc.r {
			t.Errorf("Incorrect ignoreFile() #%d (%s); E: %v, A: %v", i, tc.f, tc.r, r)
		}
	}
}

func TestDontIgnores(t *testing.T) {
	stignore := `
	!iex2
	!ign1/ex
	ign1
	i*2
	!ign2
	`
	pats := New(true)
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore", "")
	if err != nil {
		t.Fatal(err)
	}

	var tests = []struct {
		f string
		r Result
	}{
		{"ign1", Nuke},
		{"ign2", Nuke},
		{"ibla2", Nuke},
		{"iex2", DontIgnore},
		{filepath.Join("ign1", "ign"), Nuke},
		{filepath.Join("ign1", "ex"), DontIgnore},
		{filepath.Join("ign1", "iex2"), DontIgnore},
		{filepath.Join("iex2", "ign"), DontIgnore},
		{filepath.Join("foo", "bar", "ign1"), Nuke},
		{filepath.Join("foo", "bar", "ign2"), Nuke},
		{filepath.Join("foo", "bar", "iex2"), DontIgnore},
	}

	for _, tc := range tests {
		if r := pats.match(tc.f); r != tc.r {
			t.Errorf("Incorrect match for %s: %v != %v", tc.f, r, tc.r)
		}
	}
}

func TestPreserve(t *testing.T) {
	stignore := `
	(?preserve)iex2
	(?preserve)Path with Spaces
	!(?preserve)ign1/ex
	ign1
	!i*2
	!(?preserve)ign2
	(?preserve)(?i)**ifn1
	!(?preserve)(?i)**ifn2
	!(?preserve)(?i)**ifn3
	(?preserve)(?i)ifn4
	`
	pats := New(true)
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore", "")
	if err != nil {
		t.Fatal(err)
	}

	var tests = []struct {
		f string
		r Result
	}{
		{"ign1", Nuke},
		{"Path with Spaces", Preserve},
		{"ign2", DontIgnore},
		{"ibla2", DontIgnore},
		{"iex2", Preserve},
		{filepath.Join("ign1", "ign"), Nuke},
		{filepath.Join("ign1", "ex"), Preserve},
		{filepath.Join("ign1", "iex2"), Preserve},
		{filepath.Join("iex2", "ign"), Preserve},
		{filepath.Join("foo", "bar", "ign1"), Nuke},
		{filepath.Join("foo", "bar", "ign2"), DontIgnore},
		{filepath.Join("foo", "bar", "iex2"), Preserve},
		{filepath.Join("foo", "bar", "i*2"), DontIgnore},
		{filepath.Join("foo", "bar", "ifn1"), Preserve},
		{filepath.Join("foo", "bar", "iFn1"), Preserve},
		{filepath.Join("foo", "bar", "ifn2"), DontIgnore},
		{filepath.Join("foo", "bar", "ifn3"), Preserve},
		{"ifn4", Preserve},
		{filepath.Join("foo", "bar", "ifn4"), DontIgnore},
	}

	for _, tc := range tests {
		if r := pats.match(tc.f); r != tc.r {
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
		err := New(true).Parse(bytes.NewBufferString(pat), ".stignore", "")
		if err == nil {
			t.Errorf("No error for pattern %q", pat)
		}
	}
}

func TestCaseSensitivity(t *testing.T) {
	ign := New(true)
	err := ign.Parse(bytes.NewBufferString("test"), ".stignore", "")
	if err != nil {
		t.Error(err)
	}

	match := []string{"test"}
	dontMatch := []string{"foo"}

	switch runtime.GOOS {
	case "darwin", "windows":
		match = append(match, "TEST", "Test", "tESt")
	default:
		dontMatch = append(dontMatch, "TEST", "Test", "tESt")
	}

	for _, tc := range match {
		if ign.match(tc) != Nuke {
			t.Errorf("Incorrect match for %q: should be matched", tc)
		}
	}

	for _, tc := range dontMatch {
		if ign.match(tc) != DontIgnore {
			t.Errorf("Incorrect match for %q: should not be matched", tc)
		}
	}
}

func TestCaching(t *testing.T) {
	fd1, err := ioutil.TempFile("testdata", "")
	if err != nil {
		t.Fatal(err)
	}

	fd2, err := ioutil.TempFile("testdata", "")
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

	pats := New(true)
	err = pats.Load(fd1.Name(), filepath.Dir(fd1.Name()))
	if err != nil {
		t.Fatal(err)
	}

	if pats.matches.len() != 0 {
		t.Fatal("Expected empty cache")
	}

	if len(pats.patterns) != 4 {
		t.Fatal("Incorrect number of patterns loaded", len(pats.patterns), "!=", 4)
	}

	// Cache some outcomes

	for _, letter := range []string{"a", "b", "x", "y"} {
		pats.match(letter)
	}

	if pats.matches.len() != 4 {
		t.Fatal("Expected 4 cached results")
	}

	// Reload file, expect old outcomes to be preserved

	err = pats.Load(fd1.Name(), filepath.Dir(fd1.Name()))
	if err != nil {
		t.Fatal(err)
	}
	if pats.matches.len() != 4 {
		t.Fatal("Expected 4 cached results")
	}

	// Modify the include file, expect empty cache

	fd2.WriteString("/z/\n")

	err = pats.Load(fd1.Name(), filepath.Dir(fd1.Name()))
	if err != nil {
		t.Fatal(err)
	}

	if pats.matches.len() != 0 {
		t.Fatal("Expected 0 cached results")
	}

	// Cache some outcomes again

	for _, letter := range []string{"b", "x", "y"} {
		pats.match(letter)
	}

	// Verify that outcomes preserved on next laod

	err = pats.Load(fd1.Name(), filepath.Dir(fd1.Name()))
	if err != nil {
		t.Fatal(err)
	}
	if pats.matches.len() != 3 {
		t.Fatal("Expected 3 cached results")
	}

	// Modify the root file, expect cache to be invalidated

	fd1.WriteString("/a/\n")

	err = pats.Load(fd1.Name(), filepath.Dir(fd1.Name()))
	if err != nil {
		t.Fatal(err)
	}
	if pats.matches.len() != 0 {
		t.Fatal("Expected cache invalidation")
	}

	// Cache some outcomes again

	for _, letter := range []string{"b", "x", "y"} {
		pats.match(letter)
	}

	// Verify that outcomes provided on next laod

	err = pats.Load(fd1.Name(), filepath.Dir(fd1.Name()))
	if err != nil {
		t.Fatal(err)
	}
	if pats.matches.len() != 3 {
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
	pats := New(true)
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore", "")
	if err != nil {
		t.Error(err)
	}
	if len(pats.patterns) > 0 {
		t.Errorf("Expected no patterns")
	}
}

var result Result

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
	pats := New(false)
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore", "")
	if err != nil {
		b.Error(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result = pats.match("filename")
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
	pats := New(true)
	err = pats.Load(fd.Name(), filepath.Dir(fd.Name()))
	if err != nil {
		b.Fatal(err)
	}
	// Cache the outcome for "filename"
	pats.match("filename")

	// This load should now load the cached outcomes as the set of patterns
	// has not changed.
	err = pats.Load(fd.Name(), filepath.Dir(fd.Name()))
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result = pats.match("filename")
	}
}

func TestCacheReload(t *testing.T) {
	fd, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatal(err)
	}

	defer fd.Close()
	defer os.Remove(fd.Name())

	// Ignore file matches f1 and f2

	_, err = fd.WriteString("f1\nf2\n")
	if err != nil {
		t.Fatal(err)
	}

	pats := New(true)
	err = pats.Load(fd.Name(), filepath.Dir(fd.Name()))
	if err != nil {
		t.Fatal(err)
	}

	// Verify that both are ignored

	if pats.match("f1") == DontIgnore {
		t.Error("Unexpected non-match for f1")
	}
	if pats.match("f2") == DontIgnore {
		t.Error("Unexpected non-match for f2")
	}
	if pats.match("f3") == Nuke {
		t.Error("Unexpected match for f3")
	}

	// Rewrite file to match f1 and f3

	err = fd.Truncate(0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fd.Seek(0, os.SEEK_SET)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fd.WriteString("f1\nf3\n")
	if err != nil {
		t.Fatal(err)
	}

	err = pats.Load(fd.Name(), filepath.Dir(fd.Name()))
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the new patterns are in effect

	if pats.match("f1") == DontIgnore {
		t.Error("Unexpected non-match for f1")
	}
	if pats.match("f2") == Nuke {
		t.Error("Unexpected match for f2")
	}
	if pats.match("f3") == DontIgnore {
		t.Error("Unexpected non-match for f3")
	}
}

func TestHash(t *testing.T) {
	p1 := New(true)
	err := p1.Load("testdata/.stignore", "testdata")
	if err != nil {
		t.Fatal(err)
	}

	// Same list of patterns as testdata/.stignore, after expansion
	stignore := `
	dir2/dfile
	dir3
	bfile
	dir1/cfile
	**/efile
	/ffile
	lost+found
	`
	p2 := New(true)
	err = p2.Parse(bytes.NewBufferString(stignore), ".stignore", "")
	if err != nil {
		t.Fatal(err)
	}

	// Not same list of patterns
	stignore = `
	dir2/dfile
	dir3
	bfile
	dir1/cfile
	/ffile
	lost+found
	`
	p3 := New(true)
	err = p3.Parse(bytes.NewBufferString(stignore), ".stignore", "")
	if err != nil {
		t.Fatal(err)
	}

	if p1.Hash() == "" {
		t.Error("p1 hash blank")
	}
	if p2.Hash() == "" {
		t.Error("p2 hash blank")
	}
	if p3.Hash() == "" {
		t.Error("p3 hash blank")
	}
	if p1.Hash() != p2.Hash() {
		t.Error("p1-p2 hashes differ")
	}
	if p1.Hash() == p3.Hash() {
		t.Error("p1-p3 hashes same")
	}
}

func TestHashOfEmpty(t *testing.T) {
	p1 := New(true)
	err := p1.Load("testdata/.stignore", "testdata")
	if err != nil {
		t.Fatal(err)
	}

	firstHash := p1.Hash()

	// Reloading with a non-existent file should empty the patterns and
	// recalculate the hash. d41d8cd98f00b204e9800998ecf8427e is the md5 of
	// nothing.

	p1.Load("file/does/not/exist", "empty")
	secondHash := p1.Hash()

	if firstHash == secondHash {
		t.Error("hash did not change")
	}
	if secondHash != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Error("second hash is not hash of empty string")
	}
	if len(p1.patterns) != 0 {
		t.Error("there are more than zero patterns")
	}
}

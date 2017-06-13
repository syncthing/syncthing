// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package ignore

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestIgnore(t *testing.T) {
	pats := New(WithCache(true))
	err := pats.Load("testdata/.stignore")
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

		{"lost+found", true},
	}

	for i, tc := range tests {
		if r := pats.Match(tc.f); r.IsIgnored() != tc.r {
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
	pats := New(WithCache(true))
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
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
		if r := pats.Match(tc.f); r.IsIgnored() != tc.r {
			t.Errorf("Incorrect match for %s: %v != %v", tc.f, r, tc.r)
		}
	}
}

func TestFlagOrder(t *testing.T) {
	stignore := `
	## Ok cases
	(?i)(?d)!ign1
	(?d)(?i)!ign2
	(?i)!(?d)ign3
	(?d)!(?i)ign4
	!(?i)(?d)ign5
	!(?d)(?i)ign6
	## Bad cases
	!!(?i)(?d)ign7
	(?i)(?i)(?d)ign8
	(?i)(?d)(?d)!ign9
	(?d)(?d)!ign10
	`
	pats := New(WithCache(true))
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		t.Fatal(err)
	}

	for i := 1; i < 7; i++ {
		pat := fmt.Sprintf("ign%d", i)
		if r := pats.Match(pat); r.IsIgnored() || r.IsDeletable() {
			t.Errorf("incorrect %s", pat)
		}
	}
	for i := 7; i < 10; i++ {
		pat := fmt.Sprintf("ign%d", i)
		if r := pats.Match(pat); r.IsDeletable() {
			t.Errorf("incorrect %s", pat)
		}
	}

	if r := pats.Match("(?d)!ign10"); !r.IsIgnored() {
		t.Errorf("incorrect")
	}
}

func TestDeletables(t *testing.T) {
	stignore := `
	(?d)ign1
	(?d)(?i)ign2
	(?i)(?d)ign3
	!(?d)ign4
	!ign5
	!(?i)(?d)ign6
	ign7
	(?i)ign8
	`
	pats := New(WithCache(true))
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		t.Fatal(err)
	}

	var tests = []struct {
		f string
		i bool
		d bool
	}{
		{"ign1", true, true},
		{"ign2", true, true},
		{"ign3", true, true},
		{"ign4", false, false},
		{"ign5", false, false},
		{"ign6", false, false},
		{"ign7", true, false},
		{"ign8", true, false},
	}

	for _, tc := range tests {
		if r := pats.Match(tc.f); r.IsIgnored() != tc.i || r.IsDeletable() != tc.d {
			t.Errorf("Incorrect match for %s: %v != Result{%t, %t}", tc.f, r, tc.i, tc.d)
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
		err := New(WithCache(true)).Parse(bytes.NewBufferString(pat), ".stignore")
		if err == nil {
			t.Errorf("No error for pattern %q", pat)
		}
	}
}

func TestCaseSensitivity(t *testing.T) {
	ign := New(WithCache(true))
	err := ign.Parse(bytes.NewBufferString("test"), ".stignore")
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
		if !ign.Match(tc).IsIgnored() {
			t.Errorf("Incorrect match for %q: should be matched", tc)
		}
	}

	for _, tc := range dontMatch {
		if ign.Match(tc).IsIgnored() {
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

	pats := New(WithCache(true))
	err = pats.Load(fd1.Name())
	if err != nil {
		t.Fatal(err)
	}

	if pats.matches.len() != 0 {
		t.Fatal("Expected empty cache")
	}

	// Cache some outcomes

	for _, letter := range []string{"a", "b", "x", "y"} {
		pats.Match(letter)
	}

	if pats.matches.len() != 4 {
		t.Fatal("Expected 4 cached results")
	}

	// Reload file, expect old outcomes to be preserved

	err = pats.Load(fd1.Name())
	if err != nil {
		t.Fatal(err)
	}
	if pats.matches.len() != 4 {
		t.Fatal("Expected 4 cached results")
	}

	// Modify the include file, expect empty cache. Ensure the timestamp on
	// the file changes.

	fd2.WriteString("/z/\n")
	fd2.Sync()
	fakeTime := time.Now().Add(5 * time.Second)
	os.Chtimes(fd2.Name(), fakeTime, fakeTime)

	err = pats.Load(fd1.Name())
	if err != nil {
		t.Fatal(err)
	}

	if pats.matches.len() != 0 {
		t.Fatal("Expected 0 cached results")
	}

	// Cache some outcomes again

	for _, letter := range []string{"b", "x", "y"} {
		pats.Match(letter)
	}

	// Verify that outcomes preserved on next load

	err = pats.Load(fd1.Name())
	if err != nil {
		t.Fatal(err)
	}
	if pats.matches.len() != 3 {
		t.Fatal("Expected 3 cached results")
	}

	// Modify the root file, expect cache to be invalidated

	fd1.WriteString("/a/\n")
	fd1.Sync()
	fakeTime = time.Now().Add(5 * time.Second)
	os.Chtimes(fd1.Name(), fakeTime, fakeTime)

	err = pats.Load(fd1.Name())
	if err != nil {
		t.Fatal(err)
	}
	if pats.matches.len() != 0 {
		t.Fatal("Expected cache invalidation")
	}

	// Cache some outcomes again

	for _, letter := range []string{"b", "x", "y"} {
		pats.Match(letter)
	}

	// Verify that outcomes provided on next load

	err = pats.Load(fd1.Name())
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
	pats := New(WithCache(true))
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
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
	pats := New()
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		b.Error(err)
	}

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
	pats := New(WithCache(true))
	err = pats.Load(fd.Name())
	if err != nil {
		b.Fatal(err)
	}
	// Cache the outcome for "filename"
	pats.Match("filename")

	// This load should now load the cached outcomes as the set of patterns
	// has not changed.
	err = pats.Load(fd.Name())
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result = pats.Match("filename")
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

	pats := New(WithCache(true))
	err = pats.Load(fd.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Verify that both are ignored

	if !pats.Match("f1").IsIgnored() {
		t.Error("Unexpected non-match for f1")
	}
	if !pats.Match("f2").IsIgnored() {
		t.Error("Unexpected non-match for f2")
	}
	if pats.Match("f3").IsIgnored() {
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
	fd.Sync()
	fakeTime := time.Now().Add(5 * time.Second)
	os.Chtimes(fd.Name(), fakeTime, fakeTime)

	err = pats.Load(fd.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the new patterns are in effect

	if !pats.Match("f1").IsIgnored() {
		t.Error("Unexpected non-match for f1")
	}
	if pats.Match("f2").IsIgnored() {
		t.Error("Unexpected match for f2")
	}
	if !pats.Match("f3").IsIgnored() {
		t.Error("Unexpected non-match for f3")
	}
}

func TestHash(t *testing.T) {
	p1 := New(WithCache(true))
	err := p1.Load("testdata/.stignore")
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
	p2 := New(WithCache(true))
	err = p2.Parse(bytes.NewBufferString(stignore), ".stignore")
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
	p3 := New(WithCache(true))
	err = p3.Parse(bytes.NewBufferString(stignore), ".stignore")
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
	p1 := New(WithCache(true))
	err := p1.Load("testdata/.stignore")
	if err != nil {
		t.Fatal(err)
	}

	firstHash := p1.Hash()

	// Reloading with a non-existent file should empty the patterns and
	// recalculate the hash. d41d8cd98f00b204e9800998ecf8427e is the md5 of
	// nothing.

	p1.Load("file/does/not/exist")
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

func TestWindowsPatterns(t *testing.T) {
	// We should accept patterns as both a/b and a\b and match that against
	// both kinds of slash as well.
	if runtime.GOOS != "windows" {
		t.Skip("Windows specific test")
		return
	}

	stignore := `
	a/b
	c\d
	`
	pats := New(WithCache(true))
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		t.Fatal(err)
	}

	tests := []string{`a\b`, `c\d`}
	for _, pat := range tests {
		if !pats.Match(pat).IsIgnored() {
			t.Errorf("Should match %s", pat)
		}
	}
}

func TestAutomaticCaseInsensitivity(t *testing.T) {
	// We should do case insensitive matching by default on some platforms.
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("Windows/Mac specific test")
		return
	}

	stignore := `
	A/B
	c/d
	`
	pats := New(WithCache(true))
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		t.Fatal(err)
	}

	tests := []string{`a/B`, `C/d`}
	for _, pat := range tests {
		if !pats.Match(pat).IsIgnored() {
			t.Errorf("Should match %s", pat)
		}
	}
}

func TestCommas(t *testing.T) {
	stignore := `
	foo,bar.txt
	{baz,quux}.txt
	`
	pats := New(WithCache(true))
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		match bool
	}{
		{"foo.txt", false},
		{"bar.txt", false},
		{"foo,bar.txt", true},
		{"baz.txt", true},
		{"quux.txt", true},
		{"baz,quux.txt", false},
	}

	for _, tc := range tests {
		if pats.Match(tc.name).IsIgnored() != tc.match {
			t.Errorf("Match of %s was %v, should be %v", tc.name, !tc.match, tc.match)
		}
	}
}

func TestIssue3164(t *testing.T) {
	stignore := `
	(?d)(?i)*.part
	(?d)(?i)/foo
	(?d)(?i)**/bar
	`
	pats := New(WithCache(true))
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		t.Fatal(err)
	}

	expanded := pats.Patterns()
	t.Log(expanded)
	expected := []string{
		"(?d)(?i)*.part",
		"(?d)(?i)**/*.part",
		"(?d)(?i)*.part/**",
		"(?d)(?i)**/*.part/**",
		"(?d)(?i)/foo",
		"(?d)(?i)/foo/**",
		"(?d)(?i)**/bar",
		"(?d)(?i)bar",
		"(?d)(?i)**/bar/**",
		"(?d)(?i)bar/**",
	}

	if len(expanded) != len(expected) {
		t.Errorf("Unmatched count: %d != %d", len(expanded), len(expected))
	}

	for i := range expanded {
		if expanded[i] != expected[i] {
			t.Errorf("Pattern %d does not match: %s != %s", i, expanded[i], expected[i])
		}
	}
}

func TestIssue3174(t *testing.T) {
	stignore := `
	*ä*
	`
	pats := New(WithCache(true))
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		t.Fatal(err)
	}

	if !pats.Match("åäö").IsIgnored() {
		t.Error("Should match")
	}
}

func TestIssue3639(t *testing.T) {
	stignore := `
	foo/
	`
	pats := New(WithCache(true))
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		t.Fatal(err)
	}

	if !pats.Match("foo/bar").IsIgnored() {
		t.Error("Should match 'foo/bar'")
	}

	if pats.Match("foo").IsIgnored() {
		t.Error("Should not match 'foo'")
	}
}

func TestIssue3674(t *testing.T) {
	stignore := `
	a*b
	a**c
	`

	testcases := []struct {
		file    string
		matches bool
	}{
		{"ab", true},
		{"asdfb", true},
		{"ac", true},
		{"asdfc", true},
		{"as/db", false},
		{"as/dc", true},
	}

	pats := New(WithCache(true))
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range testcases {
		res := pats.Match(tc.file).IsIgnored()
		if res != tc.matches {
			t.Errorf("Matches(%q) == %v, expected %v", tc.file, res, tc.matches)
		}
	}
}

func TestGobwasGlobIssue18(t *testing.T) {
	stignore := `
	a?b
	bb?
	`

	testcases := []struct {
		file    string
		matches bool
	}{
		{"ab", false},
		{"acb", true},
		{"asdb", false},
		{"bb", false},
		{"bba", true},
		{"bbaa", false},
	}

	pats := New(WithCache(true))
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range testcases {
		res := pats.Match(tc.file).IsIgnored()
		if res != tc.matches {
			t.Errorf("Matches(%q) == %v, expected %v", tc.file, res, tc.matches)
		}
	}
}

func TestIsInternal(t *testing.T) {
	cases := []struct {
		file     string
		internal bool
	}{
		{".stfolder", true},
		{".stignore", true},
		{".stversions", true},
		{".stfolder/foo", true},
		{".stignore/foo", true},
		{".stversions/foo", true},

		{".stfolderfoo", false},
		{".stignorefoo", false},
		{".stversionsfoo", false},
		{"foo.stfolder", false},
		{"foo.stignore", false},
		{"foo.stversions", false},
		{"foo/.stfolder", false},
		{"foo/.stignore", false},
		{"foo/.stversions", false},
	}

	for _, tc := range cases {
		res := IsInternal(filepath.FromSlash(tc.file))
		if res != tc.internal {
			t.Errorf("Unexpected result: IsInteral(%q): %v should be %v", tc.file, res, tc.internal)
		}
	}
}

func TestRoot(t *testing.T) {
	stignore := `
	!/a
	/*
	`

	testcases := []struct {
		file    string
		matches bool
	}{
		{".", false},
		{"a", false},
		{"b", true},
	}

	pats := New(WithCache(true))
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range testcases {
		res := pats.Match(tc.file).IsIgnored()
		if res != tc.matches {
			t.Errorf("Matches(%q) == %v, expected %v", tc.file, res, tc.matches)
		}
	}
}

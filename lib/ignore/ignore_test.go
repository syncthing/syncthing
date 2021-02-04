// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package ignore

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/osutil"
)

func TestIgnore(t *testing.T) {
	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "testdata"))
	err := pats.Load(".stignore")
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
	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
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
	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
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
	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
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
	}

	for _, pat := range badPatterns {
		err := New(fs.NewFilesystem(fs.FilesystemTypeBasic, ".")).Parse(bytes.NewBufferString(pat), ".stignore")
		if err == nil {
			t.Errorf("No error for pattern %q", pat)
		}
		if !IsParseError(err) {
			t.Error("Should have been a parse error:", err)
		}
	}
}

func TestCaseSensitivity(t *testing.T) {
	ign := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
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
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}

	fs := fs.NewFilesystem(fs.FilesystemTypeBasic, dir)

	fd1, err := osutil.TempFile(fs, "", "")
	if err != nil {
		t.Fatal(err)
	}

	fd2, err := osutil.TempFile(fs, "", "")
	if err != nil {
		t.Fatal(err)
	}

	defer fd1.Close()
	defer fd2.Close()
	defer fs.Remove(fd1.Name())
	defer fs.Remove(fd2.Name())

	_, err = fd1.Write([]byte("/x/\n#include " + filepath.Base(fd2.Name()) + "\n"))
	if err != nil {
		t.Fatal(err)
	}

	fd2.Write([]byte("/y/\n"))

	pats := New(fs)
	err = pats.Load(fd1.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Cache some outcomes

	for _, letter := range []string{"a", "b", "x", "y"} {
		pats.Match(letter)
	}

	// Reload file, expect old outcomes to be preserved

	err = pats.Load(fd1.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Modify the include file, expect empty cache. Ensure the timestamp on
	// the file changes.

	fd2.Write([]byte("/z/\n"))
	fd2.Sync()
	fakeTime := time.Now().Add(5 * time.Second)
	fs.Chtimes(fd2.Name(), fakeTime, fakeTime)

	err = pats.Load(fd1.Name())
	if err != nil {
		t.Fatal(err)
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

	// Modify the root file, expect cache to be invalidated

	fd1.Write([]byte("/a/\n"))
	fd1.Sync()
	fakeTime = time.Now().Add(5 * time.Second)
	fs.Chtimes(fd1.Name(), fakeTime, fakeTime)

	err = pats.Load(fd1.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Verify that outcomes provided on next load

	err = pats.Load(fd1.Name())
	if err != nil {
		t.Fatal(err)
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
	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
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
	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		b.Error(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result = pats.Match("filename")
	}
}

func TestCacheReload(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}

	fs := fs.NewFilesystem(fs.FilesystemTypeBasic, dir)

	fd, err := osutil.TempFile(fs, "", "")
	if err != nil {
		t.Fatal(err)
	}

	defer fd.Close()
	defer fs.Remove(fd.Name())

	// Ignore file matches f1 and f2

	_, err = fd.Write([]byte("f1\nf2\n"))
	if err != nil {
		t.Fatal(err)
	}

	pats := New(fs)
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
	_, err = fd.Seek(0, io.SeekStart)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fd.Write([]byte("f1\nf3\n"))
	if err != nil {
		t.Fatal(err)
	}
	fd.Sync()
	fakeTime := time.Now().Add(5 * time.Second)
	fs.Chtimes(fd.Name(), fakeTime, fakeTime)

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
	p1 := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
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
	p2 := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
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
	p3 := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
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
	p1 := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
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
	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
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
	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
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
	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
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
	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
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
	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
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
	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
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

	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
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

	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
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

	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
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

func TestLines(t *testing.T) {
	stignore := `
	#include testdata/excludes

	!/a
	/*
	!/a
	`

	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		t.Fatal(err)
	}

	expectedLines := []string{
		"",
		"#include testdata/excludes",
		"",
		"!/a",
		"/*",
		"!/a",
		"",
	}

	lines := pats.Lines()
	if len(lines) != len(expectedLines) {
		t.Fatalf("len(Lines()) == %d, expected %d", len(lines), len(expectedLines))
	}
	for i := range lines {
		if lines[i] != expectedLines[i] {
			t.Fatalf("Lines()[%d] == %s, expected %s", i, lines[i], expectedLines[i])
		}
	}
}

func TestDuplicateLines(t *testing.T) {
	stignore := `
	!/a
	/*
	!/a
	`
	stignoreFiltered := `
	!/a
	/*
	`

	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "testdata"))

	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		t.Fatal(err)
	}
	patsLen := len(pats.patterns)

	err = pats.Parse(bytes.NewBufferString(stignoreFiltered), ".stignore")
	if err != nil {
		t.Fatal(err)
	}

	if patsLen != len(pats.patterns) {
		t.Fatalf("Parsed patterns differ when manually removing duplicate lines")
	}
}

func TestIssue4680(t *testing.T) {
	stignore := `
	#snapshot
	`

	testcases := []struct {
		file    string
		matches bool
	}{
		{"#snapshot", true},
		{"#snapshot/foo", true},
	}

	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
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

func TestIssue4689(t *testing.T) {
	stignore := `// orig`

	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		t.Fatal(err)
	}

	if lines := pats.Lines(); len(lines) != 1 || lines[0] != "// orig" {
		t.Fatalf("wrong lines parsing original comment:\n%q", lines)
	}

	stignore = `// new`

	err = pats.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		t.Fatal(err)
	}

	if lines := pats.Lines(); len(lines) != 1 || lines[0] != "// new" {
		t.Fatalf("wrong lines parsing changed comment:\n%v", lines)
	}
}

func TestIssue4901(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(dir)

	stignore := `
	#include unicorn-lazor-death
	puppy
	`

	if err := ioutil.WriteFile(filepath.Join(dir, ".stignore"), []byte(stignore), 0777); err != nil {
		t.Fatalf(err.Error())
	}

	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, dir))
	// Cache does not suddenly make the load succeed.
	for i := 0; i < 2; i++ {
		err := pats.Load(".stignore")
		if err == nil {
			t.Fatal("expected an error")
		}
		if fs.IsNotExist(err) {
			t.Fatal("unexpected error type")
		}
		if !IsParseError(err) {
			t.Fatal("failure to load included file should be a parse error")
		}
	}

	if err := ioutil.WriteFile(filepath.Join(dir, "unicorn-lazor-death"), []byte(" "), 0777); err != nil {
		t.Fatalf(err.Error())
	}

	err = pats.Load(".stignore")
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
}

// TestIssue5009 checks that ignored dirs are only skipped if there are no include patterns.
// https://github.com/syncthing/syncthing/issues/5009 (rc-only bug)
func TestIssue5009(t *testing.T) {
	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))

	stignore := `
	ign1
	i*2
	`
	if err := pats.Parse(bytes.NewBufferString(stignore), ".stignore"); err != nil {
		t.Fatal(err)
	}
	if !pats.skipIgnoredDirs {
		t.Error("skipIgnoredDirs should be true without includes")
	}

	stignore = `
	!iex2
	!ign1/ex
	ign1
	i*2
	!ign2
	`

	if err := pats.Parse(bytes.NewBufferString(stignore), ".stignore"); err != nil {
		t.Fatal(err)
	}

	if pats.skipIgnoredDirs {
		t.Error("skipIgnoredDirs should not be true with includes")
	}
}

func TestSpecialChars(t *testing.T) {
	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))

	stignore := `(?i)/#recycle
(?i)/#nosync
(?i)/$Recycle.bin
(?i)/$RECYCLE.BIN
(?i)/System Volume Information`
	if err := pats.Parse(bytes.NewBufferString(stignore), ".stignore"); err != nil {
		t.Fatal(err)
	}

	cases := []string{
		"#nosync",
		"$RECYCLE.BIN",
		filepath.FromSlash("$RECYCLE.BIN/S-1-5-18/desktop.ini"),
	}

	for _, c := range cases {
		if !pats.Match(c).IsIgnored() {
			t.Errorf("%q should be ignored", c)
		}
	}
}

func TestPartialIncludeLine(t *testing.T) {
	// Loading a partial #include line (no file mentioned) should error but not crash.

	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "."))
	cases := []string{
		"#include",
		"#include\n",
		"#include ",
		"#include \n",
		"#include   \n\n\n",
	}

	for _, tc := range cases {
		err := pats.Parse(bytes.NewBufferString(tc), ".stignore")
		if err == nil {
			t.Fatal("should error out")
		}
		if !IsParseError(err) {
			t.Fatal("failure to load included file should be a parse error")
		}
	}
}

func TestSkipIgnoredDirs(t *testing.T) {
	tcs := []struct {
		pattern  string
		expected bool
	}{
		{`!/test`, true},
		{`!/t[eih]t`, true},
		{`!/t*t`, true},
		{`!/t?t`, true},
		{`!/**`, true},
		{`!/parent/test`, false},
		{`!/parent/t[eih]t`, false},
		{`!/parent/t*t`, false},
		{`!/parent/t?t`, false},
		{`!/**.mp3`, false},
		{`!/pa*nt/test`, false},
		{`!/pa[sdf]nt/t[eih]t`, false},
		{`!/lowest/pa[sdf]nt/test`, false},
		{`!/lo*st/parent/test`, false},
		{`/pa*nt/test`, true},
		{`test`, true},
		{`*`, true},
	}

	for _, tc := range tcs {
		pats, err := parseLine(tc.pattern)
		if err != nil {
			t.Error(err)
		}
		for _, pat := range pats {
			if got := pat.allowsSkippingIgnoredDirs(); got != tc.expected {
				t.Errorf(`Pattern "%v": got %v, expected %v`, pat, got, tc.expected)
			}
		}
	}

	pats := New(fs.NewFilesystem(fs.FilesystemTypeBasic, "testdata"))

	stignore := `
	/foo/ign*
	!/f*
	!/bar
	*
	`
	if err := pats.Parse(bytes.NewBufferString(stignore), ".stignore"); err != nil {
		t.Fatal(err)
	}
	if !pats.SkipIgnoredDirs() {
		t.Error("SkipIgnoredDirs should be true")
	}

	stignore = `
	!/foo/ign*
	*
	`
	if err := pats.Parse(bytes.NewBufferString(stignore), ".stignore"); err != nil {
		t.Fatal(err)
	}
	if pats.SkipIgnoredDirs() {
		t.Error("SkipIgnoredDirs should be false")
	}
}

func TestEmptyPatterns(t *testing.T) {
	// These patterns are all invalid and should be rejected as such (without panicking...)
	tcs := []string{
		"!",
		"(?d)",
		"(?i)",
	}

	for _, tc := range tcs {
		m := New(fs.NewFilesystem(fs.FilesystemTypeFake, ""))
		err := m.Parse(strings.NewReader(tc), ".stignore")
		if err == nil {
			t.Error("Should reject invalid pattern", tc)
		}
		if !IsParseError(err) {
			t.Fatal("bad pattern should be a parse error")
		}
	}
}

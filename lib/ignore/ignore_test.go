// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package ignore

import (
	"bytes"
	"path/filepath"
	"runtime"
	"testing"
)

func TestIgnore(t *testing.T) {
	pats := New()
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
	pats := New()
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
		err := New().Parse(bytes.NewBufferString(pat), ".stignore")
		if err == nil {
			t.Errorf("No error for pattern %q", pat)
		}
	}
}

func TestCaseSensitivity(t *testing.T) {
	ign := New()
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
	pats := New()
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		t.Error(err)
	}
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

func TestHash(t *testing.T) {
	p1 := New()
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
	p2 := New()
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
	p3 := New()
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
	p1 := New()
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
	pats := New()
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		t.Fatal(err)
	}

	tests := []string{`a\b`, `c\d`}
	for _, pat := range tests {
		if !pats.Match(pat) {
			t.Errorf("Should match %s", pat)
		}
	}
}

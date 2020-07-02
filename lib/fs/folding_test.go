// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	// "io/ioutil"
	// "os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnicodeLowercase(t *testing.T) {
	cases := [][2]string{
		{"", ""},
		{"hej", "hej"},
		{"HeJ!@#", "hej!@#"},
		// Western Europe diacritical stuff is trivial
		{"ÜBERRÄKSMÖRGÅS", "überräksmörgås"},
		// Cyrillic seems regular as well
		{"Привет", "привет"},
		// Greek has multiple lower case characters for things depending on
		// context; we should always choose the right one.
		{"Ὀδυσσεύς", "ὀδυσσεύσ"},
		{"ὈΔΥΣΣΕΎΣ", "ὀδυσσεύσ"},
		// German ß doesn't really have an upper case variant, and we
		// shouldn't mess things up when lower casing it either. We don't
		// attempt to make ß equivalent to "ss".
		{"Reichwaldstraße", "reichwaldstraße"},
		// The Turks do their thing with the Is.... Like the Greek example
		// we pick just the one canonicalized "i" although you can argue
		// with this... From what I understand most operating systems don't
		// get this right anyway.
		{"İI", "ii"},
		// Arabic doesn't do case folding.
		{"العَرَبِيَّة", "العَرَبِيَّة"},
		// Neither does Hebrew.
		{"עברית", "עברית"},
		// Nor Chinese, in any variant.
		{"汉语/漢語 or 中文", "汉语/漢語 or 中文"},
		// Niether katakana as far as I can tell.
		{"チャーハン", "チャーハン"},
		// Some special unicode characters, however, are folded by OSes
		{"\u212A", "k"},
	}
	for _, tc := range cases {
		res := UnicodeLowercase(tc[0])
		if res != tc[1] {
			t.Errorf("UnicodeLowercase(%q) => %q, expected %q", tc[0], res, tc[1])
		}
	}
}

func TestRealCase(t *testing.T) {
	comps := []string{"Foo", "bar", "BAZ", "bAs"}
	path := filepath.Join(comps...)
	fsRoot := "foo?insens="
	for _, insens := range []string{"true", "false"} {
		testFs := NewFilesystem(FilesystemTypeFake, fsRoot+insens)
		testFs.MkdirAll(filepath.Join(comps[:len(comps)-1]...), 0777)
		fd, err := testFs.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		fd.Close()
		testRealCase(t, testFs, path, comps)
	}
}

func testRealCase(t *testing.T, testFs Filesystem, path string, comps []string) {
	rc := NewCachedRealCaser(testFs)
	t.Helper()
	for i, tc := range []struct {
		in  string
		len int
	}{
		{path, 4},
		{strings.ToLower(path), 4},
		{strings.ToUpper(path), 4},
		{"foo", 1},
		{"FOO", 1},
		{"foO", 1},
		{filepath.Join("Foo", "bar"), 2},
		{filepath.Join("Foo", "bAr"), 2},
		{filepath.Join("FoO", "bar"), 2},
		{filepath.Join("foo", "bar", "BAZ"), 3},
		{filepath.Join("Foo", "bar", "bAz"), 3},
		{filepath.Join("foo", "bar", "BAZ"), 3}, // Repeat on purpose
	} {
		out, err := rc.RealCase(tc.in)
		if err != nil {
			t.Error(err)
		} else if exp := filepath.Join(comps[:tc.len]...); out != exp {
			t.Errorf("tc %v: Expected %v, got %v", i, exp, out)
		}
	}
}

func TestRealCaseSensitive(t *testing.T) {
	testFs := NewFilesystem(FilesystemTypeFake, "foo?insens=false")
	// tmpDir, err := ioutil.TempDir("", "syncthing_TestRealCaseSensitive-")
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// testFs := NewFilesystem(FilesystemTypeBasic, tmpDir)
	// defer os.RemoveAll(tmpDir)

	names := make([]string, 2)
	names[0] = "foo"
	names[1] = strings.ToUpper(names[0])
	for _, n := range names {
		if err := testFs.MkdirAll(n, 0777); err != nil {
			t.Fatal(err)
		}
	}

	rc := NewCachedRealCaser(testFs)
	for _, n := range names {
		if rn, err := rc.RealCase(n); err != nil {
			t.Error(err)
		} else if rn != n {
			t.Errorf("Got %v, expected %v", rn, n)
		}
	}
}

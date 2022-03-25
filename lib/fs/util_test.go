// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"math/rand"
	"runtime"
	"testing"
	"unicode"
	"unicode/utf8"
)

func TestCommonPrefix(t *testing.T) {
	test := func(first, second, expect string) {
		t.Helper()
		res := CommonPrefix(first, second)
		if res != expect {
			t.Errorf("Expected %s got %s", expect, res)
		}
	}

	if runtime.GOOS == "windows" {
		test(`c:\Audrius\Downloads`, `c:\Audrius\Docs`, `c:\Audrius`)
		test(`c:\Audrius\Downloads`, `C:\Audrius\Docs`, ``) // Case differences :(
		test(`C:\Audrius-a\Downloads`, `C:\Audrius-b\Docs`, `C:\`)
		test(`\\?\C:\Audrius-a\Downloads`, `\\?\C:\Audrius-b\Docs`, `\\?\C:\`)
		test(`\\?\C:\Audrius\Downloads`, `\\?\C:\Audrius\Docs`, `\\?\C:\Audrius`)
		test(`Audrius-a\Downloads`, `Audrius-b\Docs`, ``)
		test(`Audrius\Downloads`, `Audrius\Docs`, `Audrius`)
		test(`c:\Audrius\Downloads`, `Audrius\Docs`, ``)
		test(`c:\`, `c:\`, `c:\`)
		test(`\\?\c:\`, `\\?\c:\`, `\\?\c:\`)
	} else {
		test(`/Audrius/Downloads`, `/Audrius/Docs`, `/Audrius`)
		test(`/Audrius\Downloads`, `/Audrius\Docs`, `/`)
		test(`/Audrius-a/Downloads`, `/Audrius-b/Docs`, `/`)
		test(`Audrius\Downloads`, `Audrius\Docs`, ``) // Windows separators
		test(`Audrius/Downloads`, `Audrius/Docs`, `Audrius`)
		test(`Audrius-a\Downloads`, `Audrius-b\Docs`, ``)
		test(`/Audrius/Downloads`, `Audrius/Docs`, ``)
		test(`/`, `/`, `/`)
	}
	test(`Audrius`, `Audrius`, `Audrius`)
	test(`.`, `.`, `.`)
}

func TestWindowsInvalidFilename(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{`asdf.txt`, nil},
		{`nul`, errInvalidFilenameWindowsReservedName},
		{`nul.txt`, errInvalidFilenameWindowsReservedName},
		{`nul.jpg.txt`, errInvalidFilenameWindowsReservedName},
		{`some.nul.jpg`, nil},
		{`foo>bar.txt`, errInvalidFilenameWindowsReservedChar},
		{`foo \bar.txt`, errInvalidFilenameWindowsSpacePeriod},
		{`foo.\bar.txt`, errInvalidFilenameWindowsSpacePeriod},
		{`foo.d\bar.txt`, nil},
		{`foo.d\bar .txt`, nil},
		{`foo.d\bar. txt`, nil},
	}

	for _, tc := range cases {
		err := WindowsInvalidFilename(tc.name)
		if err != tc.err {
			t.Errorf("For %q, got %v, expected %v", tc.name, err, tc.err)
		}
	}
}

func TestSanitizePath(t *testing.T) {
	cases := [][2]string{
		{"", ""},
		{"foo", "foo"},
		{`\*/foo\?/bar[{!@$%^&*#()}]`, "foo bar ()"},
		{"Räksmörgås", "Räksmörgås"},
		{`Räk \/ smörgås`, "Räk smörgås"},
		{"هذا هو *\x07?اسم الملف", "هذا هو اسم الملف"},
		{`../foo.txt`, `.. foo.txt`},
		{"  \t \n filename in  \t space\r", "filename in space"},
		{"你\xff好", `你 好`},
		{"\000 foo", "foo"},
	}

	for _, tc := range cases {
		res := SanitizePath(tc[0])
		if res != tc[1] {
			t.Errorf("SanitizePath(%q) => %q, expected %q", tc[0], res, tc[1])
		}
	}
}

// Fuzz test: SanitizePath must always return strings of printable UTF-8
// characters when fed random data.
//
// Note that space is considered printable, but other whitespace runes are not.
func TestSanitizePathFuzz(t *testing.T) {
	buf := make([]byte, 128)

	for i := 0; i < 100; i++ {
		rand.Read(buf)
		path := SanitizePath(string(buf))
		if !utf8.ValidString(path) {
			t.Errorf("SanitizePath(%q) => %q, not valid UTF-8", buf, path)
			continue
		}
		for _, c := range path {
			if !unicode.IsPrint(c) {
				t.Errorf("non-printable rune %q in sanitized path", c)
			}
		}
	}
}

func benchmarkWindowsInvalidFilename(b *testing.B, name string) {
	for i := 0; i < b.N; i++ {
		WindowsInvalidFilename(name)
	}
}
func BenchmarkWindowsInvalidFilenameValid(b *testing.B) {
	benchmarkWindowsInvalidFilename(b, "License.txt.gz")
}
func BenchmarkWindowsInvalidFilenameNUL(b *testing.B) {
	benchmarkWindowsInvalidFilename(b, "nul.txt.gz")
}

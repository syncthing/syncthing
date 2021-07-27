// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"io/ioutil"
	"os"
	"testing"
)

var encodedNameCases = [][2]string{
	{"", ""},
	{" ", "\uf020"},
	{".", "."},
	{"..", ".."},
	{"...", "\uf02e\uf02e\uf02e"},
	{"::", "\uf03a\uf03a"},
	{"[:", "[\uf03a"},
	{":[", "\uf03a["},

	{"a ", "a\uf020"},
	{"a.", "a\uf02e"},
	{"a  ", "a\uf020\uf020"},
	{"a. ", "a\uf02e\uf020"},
	{"a .", "a\uf020\uf02e"},
	{"o k", "o k"},
	{"o.k", "o.k"},
	{" ok", " ok"},
	{".ok", ".ok"},
}

var encodedPathCases = [][2]string{
	{`\`, `\`},
	{`\\`, `\\`},
	{`\.`, `\.`},
	{`\..`, `\..`},
	{`.\`, `.\`},
	{`..\`, `..\`},
	{"a", "a"},
	{`a\`, `a\`},
	{`\a`, `\a`},
	{`\a\`, `\a\`},
	{`\ \ `, "\\\uf020\\\uf020"},
	{"c:", "c:"},
	{"c:a", "c:a"},
	{`c:a\`, `c:a\`},
	{"c: ", "c:\uf020"},
	{"c:.", "c:."},
	{"c:..", "c:.."},
	{"c:...", "c:\uf02e\uf02e\uf02e"},
	{`c:\`, `c:\`},
	{`c:\\`, `c:\\`},
	{`c:\ `, "c:\\\uf020"},
	{`c:\.`, `c:\.`},
	{`c:\..`, `c:\..`},
	{`c:\...`, "c:\\\uf02e\uf02e\uf02e"},
	{`c:\ \ `, "c:\\\uf020\\\uf020"},

	{`\\?\c:\`, `\\?\c:\`},
	{`\\?\c:\\`, `\\?\c:\\`},
	{`\\?\c:\ `, "\\\\?\\c:\\\uf020"},
	{`\\?\c:\.`, `\\?\c:\.`},
	{`\\?\c:\..`, `\\?\c:\..`},
	{`\\?\c:\...`, "\\\\?\\c:\\\uf02e\uf02e\uf02e"},
	{`\\?\c:\ \ `, "\\\\?\\c:\\\uf020\\\uf020"},

	{"[:", "[\uf03a"},
	{"[:a", "[\uf03aa"},
	{`[:a\`, "[\uf03aa\\"},
	{"[: ", "[\uf03a\uf020"},
	{"[:.", "[\uf03a\uf02e"},
	{"[:..", "[\uf03a\uf02e\uf02e"},
	{"[:...", "[\uf03a\uf02e\uf02e\uf02e"},
	{`[:\`, "[\uf03a\\"},
	{`[:\\`, "[\uf03a\\\\"},
	{`[:\ `, "[\uf03a\\\uf020"},
	{`[:\.`, "[\uf03a\\."},
	{`[:\..`, "[\uf03a\\.."},
	{`[:\...`, "[\uf03a\\\uf02e\uf02e\uf02e"},
	{`[:\ \ `, "[\uf03a\\\uf020\\\uf020"},
}

func testEncoderSetup(t *testing.T) (*WindowsEncoderFilesystem, string) {
	t.Helper()
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}

	return newWindowsEncoderFilesystem(newBasicFilesystem(dir)), dir
}

func TestEncoderInvalidName(t *testing.T) {
	if pathSeparatorString != `\` {
		t.Skip("Path separator is " + pathSeparatorString)
	}
	fs, dir := testEncoderSetup(t)
	defer os.RemoveAll(dir)

	for _, r := range fs.reservedChars {
		tc := string(r)
		expected := string(r | 0xf000)
		res := fs.encodedName(tc)
		if res != expected {
			t.Errorf("encodedName(%q) => %q, expected %q", tc, res, expected)
		}
	}

	for _, tc := range encodedNameCases {
		res := fs.encodedName(tc[0])
		if res != tc[1] {
			t.Errorf("encodedName(%q) => %q, expected %q", tc[0], res, tc[1])
		}
	}
}

var sep string = pathSeparatorString

func TestEncoderInvalidPath(t *testing.T) {
	if pathSeparatorString != `\` {
		t.Skip("Path separator is " + pathSeparatorString)
	}
	fs, dir := testEncoderSetup(t)
	defer os.RemoveAll(dir)

	for _, r := range fs.reservedChars {
		s := string(r)
		encoded := string(r | 0xf000)
		tcs := map[string]string{
			s:             encoded,
			sep + s:       sep + encoded,
			s + sep:       encoded + sep,
			sep + s + sep: sep + encoded + sep,
		}
		for tc, expected := range tcs {
			res := fs.encodedName(tc)
			if res != expected {
				t.Errorf("encodedName(%q) => %q, expected %q", tc, res, expected)
			}
		}
	}

	for _, tc := range encodedPathCases {
		res := fs.encodedPath(tc[0])
		if res != tc[1] {
			t.Errorf("encodedPath(%q) => %q, expected %q", tc[0], res, tc[1])
		}
	}

	for _, tc := range encodedNameCases {
		res := fs.encodedPath(tc[0])
		if res != tc[1] {
			t.Errorf("encodedPath(%q) => %q, expected %q", tc[0], res, tc[1])
		}
	}
}

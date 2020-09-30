// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"runtime"
	"testing"
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

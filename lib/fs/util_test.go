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
	test := func(paths ...string) {
		res := CommonPrefix(paths[:len(paths)-1]...)
		expect := paths[len(paths)-1]
		if res != expect {
			t.Errorf("Expected %s got %s", expect, res)
		}
	}

	if runtime.GOOS == "windows" {
		test(`c:\Audrius\Downloads`, `c:\Audrius\Docs`, `c:\Audrius\`)
		test(`c:\Audrius\Downloads`, `C:\Audrius\Docs`, ``) // Case differences :(
		test(`c:\Audrius-a\Downloads`, `c:\Audrius-b\Docs`, `c:\`)
	} else {
		test(`/Audrius/Downloads`, `/Audrius/Docs`, `/Audrius/`)
		test(`/Audrius\Downloads`, `/audrius\Docs`, `/`)
		test(`/Audrius-a/Downloads`, `/Audrius-b/Docs`, `/`)
	}
}

// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil_test

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/osutil"
)

func TestRealCaseSensitive(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "syncthing_TestRealCaseSensitive-")
	if err != nil {
		t.Fatal(err)
	}
	testFs := fs.NewFilesystem(fs.FilesystemTypeBasic, tmpDir)
	defer os.RemoveAll(tmpDir)

	names := make([]string, 2)
	names[0] = "foo"
	names[1] = strings.ToUpper(names[0])
	for _, n := range names {
		if err := testFs.MkdirAll(n, 0777); err != nil {
			t.Fatal(err)
		}
	}
	dirNames, err := testFs.DirNames(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(dirNames) == 1 {
		t.Skip("Filesystem is case-insensitive")
	}

	for _, n := range names {
		if rn, err := osutil.RealCase(testFs, n); err != nil {
			t.Error(err)
		} else if rn != n {
			t.Errorf("Got %v, expected %v", rn, n)
		}
	}
}

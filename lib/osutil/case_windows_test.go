// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/osutil"
)

func TestRealCase(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "syncthing_TestRealCase-")
	if err != nil {
		t.Fatal(err)
	}
	testFs := fs.NewFilesystem(fs.FilesystemTypeBasic, tmpDir)
	defer os.RemoveAll(tmpDir)

	comps := []string{"Foo", "bar", "BAZ", "bAs"}
	path := filepath.Join(comps...)
	testFs.MkdirAll(filepath.Join(comps[:len(comps)-1]...), 0777)
	fd, err := testFs.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

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
	} {
		out, err := osutil.RealCase(tmpDir, tc.in)
		if err != nil {
			t.Error(err)
		} else if exp := filepath.Join(comps[:tc.len]...); out != exp {
			t.Errorf("tc %v: Expected %v, got %v", i, exp, out)
		}
	}
}

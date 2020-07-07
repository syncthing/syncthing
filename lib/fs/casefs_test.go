// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRealCase(t *testing.T) {
	bfs, tmpDir := setup(t)
	testFs := NewCaseFilesystem(bfs).(*caseFilesystem)
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
		{filepath.Join("foo", "bar", "BAZ"), 3}, // Repeat on purpose
	} {
		out, err := testFs.realCase(tc.in)
		if err != nil {
			t.Error(err)
		} else if exp := filepath.Join(comps[:tc.len]...); out != exp {
			t.Errorf("tc %v: Expected %v, got %v", i, exp, out)
		}
	}
}

func TestRealCaseSensitive(t *testing.T) {
	bfs, tmpDir := setup(t)
	testFs := NewCaseFilesystem(bfs).(*caseFilesystem)
	defer os.RemoveAll(tmpDir)

	names := make([]string, 2)
	names[0] = "foo"
	names[1] = strings.ToUpper(names[0])
	for _, n := range names {
		if err := testFs.MkdirAll(n, 0777); err != nil {
			if IsErrCaseConflict(err) {
				t.Skip("Filesystem is case-insensitive")
			}
			t.Fatal(err)
		}
	}

	for _, n := range names {
		if rn, err := testFs.realCase(n); err != nil {
			t.Error(err)
		} else if rn != n {
			t.Errorf("Got %v, expected %v", rn, n)
		}
	}
}

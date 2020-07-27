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
	// Verify realCase lookups on various underlying filesystems.

	t.Run("fake-sensitive", func(t *testing.T) {
		testRealCase(t, newFakeFilesystem(t.Name()))
	})
	t.Run("fake-insensitive", func(t *testing.T) {
		testRealCase(t, newFakeFilesystem(t.Name()+"?insens=true"))
	})
	t.Run("actual", func(t *testing.T) {
		fsys, tmpDir := setup(t)
		defer os.RemoveAll(tmpDir)
		testRealCase(t, fsys)
	})
}

func testRealCase(t *testing.T, fsys Filesystem) {
	testFs := NewCaseFilesystem(fsys).(*caseFilesystem)
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
	// Verify that realCase returns the best on-disk case for case sensitive
	// systems. Test is skipped if the underlying fs is insensitive.

	t.Run("fake-sensitive", func(t *testing.T) {
		testRealCaseSensitive(t, newFakeFilesystem(t.Name()))
	})
	t.Run("actual", func(t *testing.T) {
		fsys, tmpDir := setup(t)
		defer os.RemoveAll(tmpDir)
		testRealCaseSensitive(t, fsys)
	})
}

func testRealCaseSensitive(t *testing.T, fsys Filesystem) {
	testFs := NewCaseFilesystem(fsys).(*caseFilesystem)

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

func TestCaseFSStat(t *testing.T) {
	// Verify that a Stat() lookup behaves in a case sensitive manner
	// regardless of the underlying fs.

	t.Run("fake-sensitive", func(t *testing.T) {
		testCaseFSStat(t, newFakeFilesystem(t.Name()))
	})
	t.Run("fake-insensitive", func(t *testing.T) {
		testCaseFSStat(t, newFakeFilesystem(t.Name()+"?insens=true"))
	})
	t.Run("actual", func(t *testing.T) {
		fsys, tmpDir := setup(t)
		defer os.RemoveAll(tmpDir)
		testCaseFSStat(t, fsys)
	})
}

func testCaseFSStat(t *testing.T, fsys Filesystem) {
	fd, err := fsys.Create("foo")
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	// Check if the underlying fs is sensitive or not
	sensitive := true
	if _, err = fsys.Stat("FOO"); err == nil {
		sensitive = false
	}

	testFs := NewCaseFilesystem(fsys)
	_, err = testFs.Stat("FOO")
	if sensitive {
		if IsNotExist(err) {
			t.Log("pass: case sensitive underlying fs")
		} else {
			t.Error("expected NotExist, not", err, "for sensitive fs")
		}
	} else if IsErrCaseConflict(err) {
		t.Log("pass: case insensitive underlying fs")
	} else {
		t.Error("expected ErrCaseConflict, not", err, "for insensitive fs")
	}
}

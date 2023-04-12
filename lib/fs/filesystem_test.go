// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"path/filepath"
	"testing"

	"github.com/syncthing/syncthing/lib/build"
)

func TestIsInternal(t *testing.T) {
	cases := []struct {
		file     string
		internal bool
	}{
		{".stfolder", true},
		{".stignore", true},
		{".stversions", true},
		{".stfolder/foo", true},
		{".stignore/foo", true},
		{".stversions/foo", true},

		{".stfolderfoo", false},
		{".stignorefoo", false},
		{".stversionsfoo", false},
		{"foo.stfolder", false},
		{"foo.stignore", false},
		{"foo.stversions", false},
		{"foo/.stfolder", false},
		{"foo/.stignore", false},
		{"foo/.stversions", false},
	}

	for _, tc := range cases {
		res := IsInternal(filepath.FromSlash(tc.file))
		if res != tc.internal {
			t.Errorf("Unexpected result: IsInternal(%q): %v should be %v", tc.file, res, tc.internal)
		}
	}
}

func TestCanonicalize(t *testing.T) {
	type testcase struct {
		path     string
		expected string
		ok       bool
	}
	cases := []testcase{
		// Valid cases
		{"/bar", "bar", true},
		{"/bar/baz", "bar/baz", true},
		{"bar", "bar", true},
		{"bar/baz", "bar/baz", true},

		// Not escape attempts, but oddly formatted relative paths
		{"", ".", true},
		{"/", ".", true},
		{"/..", ".", true},
		{"./bar", "bar", true},
		{"./bar/baz", "bar/baz", true},
		{"bar/../baz", "baz", true},
		{"/bar/../baz", "baz", true},
		{"./bar/../baz", "baz", true},

		// Results in an allowed path, but does it by probing. Disallowed.
		{"../foo", "", false},
		{"../foo/bar", "", false},
		{"../foo/bar", "", false},
		{"../../baz/foo/bar", "", false},
		{"bar/../../foo/bar", "", false},
		{"bar/../../../baz/foo/bar", "", false},

		// Escape attempts.
		{"..", "", false},
		{"../", "", false},
		{"../bar", "", false},
		{"../foobar", "", false},
		{"bar/../../quux/baz", "", false},
	}

	for _, tc := range cases {
		res, err := Canonicalize(tc.path)
		if tc.ok {
			if err != nil {
				t.Errorf("Unexpected error for Canonicalize(%q): %v", tc.path, err)
				continue
			}
			exp := filepath.FromSlash(tc.expected)
			if res != exp {
				t.Errorf("Unexpected result for Canonicalize(%q): %q != expected %q", tc.path, res, exp)
			}
		} else if err == nil {
			t.Errorf("Unexpected pass for Canonicalize(%q) => %q", tc.path, res)
			continue
		}
	}
}

func TestFileModeString(t *testing.T) {
	var fm FileMode = 0777
	exp := "-rwxrwxrwx"
	if fm.String() != exp {
		t.Fatalf("Got %v, expected %v", fm.String(), exp)
	}
}

func TestIsParent(t *testing.T) {
	test := func(path, parent string, expected bool) {
		t.Helper()
		path = filepath.FromSlash(path)
		parent = filepath.FromSlash(parent)
		if res := IsParent(path, parent); res != expected {
			t.Errorf(`Unexpected result: IsParent("%v", "%v"): %v should be %v`, path, parent, res, expected)
		}
	}
	testBoth := func(path, parent string, expected bool) {
		t.Helper()
		test(path, parent, expected)
		if build.IsWindows {
			test("C:/"+path, "C:/"+parent, expected)
		} else {
			test("/"+path, "/"+parent, expected)
		}
	}

	// rel - abs
	for _, parent := range []string{"/", "/foo", "/foo/bar"} {
		for _, path := range []string{"", ".", "foo", "foo/bar", "bas", "bas/baz"} {
			if build.IsWindows {
				parent = "C:/" + parent
			}
			test(parent, path, false)
			test(path, parent, false)
		}
	}

	// equal
	for i, path := range []string{"/", "/foo", "/foo/bar", "", ".", "foo", "foo/bar"} {
		if i < 3 && build.IsWindows {
			path = "C:" + path
		}
		test(path, path, false)
	}

	test("", ".", false)
	test(".", "", false)
	for _, parent := range []string{"", "."} {
		for _, path := range []string{"foo", "foo/bar"} {
			test(path, parent, true)
			test(parent, path, false)
		}
	}
	for _, parent := range []string{"foo", "foo/bar"} {
		for _, path := range []string{"bar", "bar/foo"} {
			testBoth(path, parent, false)
			testBoth(parent, path, false)
		}
	}
	for _, parent := range []string{"foo", "foo/bar"} {
		for _, path := range []string{"foo/bar/baz", "foo/bar/baz/bas"} {
			testBoth(path, parent, true)
			testBoth(parent, path, false)
			if build.IsWindows {
				test("C:/"+path, "D:/"+parent, false)
			}
		}
	}
}

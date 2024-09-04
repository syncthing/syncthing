// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

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

func TestCaseFSMtimeFSInteraction(t *testing.T) {
	fs := NewFilesystem(FilesystemTypeFake, fmt.Sprintf("%v?insens=true&timeprecisionsecond=true", t.Name()), &OptionDetectCaseConflicts{}, NewMtimeOption(make(mapStore)))

	name := "Testfile"
	nameLower := UnicodeLowercaseNormalized(name)

	file, err := fs.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	testTime := time.Unix(1723491493, 123456789)
	err = fs.Chtimes(name, testTime, testTime)
	if err != nil {
		t.Fatal(err)
	}
	info, err := fs.Lstat(name)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(testTime) {
		t.Errorf("Expected mtime %v for %v, got %v", testTime, name, info.ModTime())
	}
	info, err = fs.Lstat(nameLower)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(testTime) {
		t.Errorf("Expected mtime %v for %v, got %v", testTime, nameLower, info.ModTime())
	}
}

func newOldFilesystem(fsType FilesystemType, uri string, opts ...Option) Filesystem {
	var caseOpt Option
	var mtimeOpt Option
	i := 0
	for _, opt := range opts {
		if caseOpt != nil && mtimeOpt != nil {
			break
		}
		switch opt.(type) {
		case *OptionDetectCaseConflicts:
			caseOpt = opt
		case *optionMtime:
			mtimeOpt = opt
		default:
			opts[i] = opt
			i++
		}
	}
	opts = opts[:i]

	var fs Filesystem
	switch fsType {
	case FilesystemTypeBasic:
		fs = newBasicFilesystem(uri, opts...)
	case FilesystemTypeFake:
		fs = newFakeFilesystem(uri, opts...)
	default:
		l.Debugln("Unknown filesystem", fsType, uri)
		fs = &errorFilesystem{
			fsType: fsType,
			uri:    uri,
			err:    errors.New("filesystem with type " + fsType.String() + " does not exist."),
		}
	}

	// Case handling is the innermost, as any filesystem calls by wrappers should be case-resolved
	if caseOpt != nil {
		fs = caseOpt.apply(fs)
	}

	// mtime handling should happen inside walking, as filesystem calls while
	// walking should be mtime-resolved too
	if mtimeOpt != nil {
		fs = mtimeOpt.apply(fs)
	}

	fs = &metricsFS{next: fs}

	if l.ShouldDebug("walkfs") {
		return NewWalkFilesystem(newLogFilesystem(fs, 1))
	}

	if l.ShouldDebug("fs") {
		return newLogFilesystem(NewWalkFilesystem(fs), 1)
	}

	return fs
}

func TestRepro9677(t *testing.T) {
	mtimeDB := make(mapStore)
	name := "Testfile"
	nameLower := UnicodeLowercaseNormalized(name)
	testTime := time.Unix(1723491493, 123456789)

	// Start with a "fs-wrapping" as it was before moving case-fs to the
	// outermost layer:
	oldFS := newOldFilesystem(FilesystemTypeFake, fmt.Sprintf("%v?insens=true&timeprecisionsecond=true", t.Name()), &OptionDetectCaseConflicts{}, NewMtimeOption(mtimeDB))

	// Create a file, set it's mtime and check that we get the expected mtime when stat-ing.
	file, err := oldFS.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	err = oldFS.Chtimes(name, testTime, testTime)
	if err != nil {
		t.Fatal(err)
	}

	checkMtime := func(fs Filesystem) {
		t.Helper()
		info, err := fs.Lstat(name)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(testTime) {
			t.Errorf("Expected mtime %v for %v, got %v", testTime, name, info.ModTime())
		}
		info, err = fs.Lstat(nameLower)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(testTime) {
			t.Errorf("Expected mtime %v for %v, got %v", testTime, nameLower, info.ModTime())
		}
	}

	checkMtime(oldFS)

	// Now we switch to the new filesystem, basically simulating an upgrade. We
	// use the same backing map for the mtime DB, which is equivalent (hopefully
	// equivalent enough) to reality where this info is persisted in the main
	// DB.
	// outermost layer:
	newFS := NewFilesystem(FilesystemTypeFake, fmt.Sprintf("%v?insens=true&timeprecisionsecond=true", t.Name()), &OptionDetectCaseConflicts{}, NewMtimeOption(mtimeDB))

	checkMtime(newFS)
}

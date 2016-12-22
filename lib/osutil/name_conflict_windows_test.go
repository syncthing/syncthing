// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build windows

package osutil_test

import (
	"os"
	"path"
	"path/filepath"
	"syscall"
	"testing"
	"unsafe"

	"github.com/syncthing/syncthing/lib/osutil"
)

func TestCheckNameConflictCasing(t *testing.T) {
	os.RemoveAll("testdata")
	defer os.RemoveAll("testdata")
	os.MkdirAll("testdata/Foo/BAR/baz", 0755)
	// check if the file system is case-sensitive
	if _, err := os.Lstat("testdata/foo"); err != nil {
		t.Skip("pointless test")
		return
	}

	cases := []struct {
		name         string
		conflictFree bool
	}{
		// Exists
		{"Foo", true},
		{"Foo/BAR", true},
		{"Foo/BAR/baz", true},
		// Doesn't exist
		{"bar", true},
		{"Foo/baz", true},
		// Conflicts
		{"foo", false},
		{"foo/BAR", false},
		{"Foo/bar", false},
		{"Foo/BAR/BAZ", false},
	}

	for _, tc := range cases {
		nativeName := filepath.FromSlash(tc.name)
		if res := osutil.CheckNameConflict("testdata", nativeName); res != tc.conflictFree {
			t.Errorf("CheckNameConflict(%q) = %v, should be %v", tc.name, res, tc.conflictFree)
		}
	}
}

func TestCheckNameConflictShortName(t *testing.T) {
	os.RemoveAll("testdata")
	defer os.RemoveAll("testdata")
	os.MkdirAll("testdata/foobarbaz/qux", 0755)
	ppath, err := syscall.UTF16PtrFromString("testdata/foobarbaz")
	if err != nil {
		t.Fatal("unexpected error", err)
	}
	// check if the file system supports short names
	bufferSize, err := syscall.GetShortPathName(ppath, nil, 0)
	if err != nil {
		t.Skip("pointless test")
		return
	}

	// get the short name
	buffer := make([]uint16, bufferSize)
	length, err := syscall.GetShortPathName(ppath,
		(*uint16)(unsafe.Pointer(&buffer[0])), bufferSize)
	if err != nil {
		t.Fatal("unexpected error", err)
	}
	// on success length doesn't contain the terminating null character
	if bufferSize != length+1 {
		t.Fatal("length of short name changed")
	}
	shortName := filepath.Base(syscall.UTF16ToString(buffer))

	cases := []struct {
		name         string
		conflictFree bool
	}{
		// Exists
		{"foobarbaz", true},
		{"foobarbaz/qux", true},
		// Doesn't exist
		{"foo", true},
		{"foobarbaz/quux", true},
		// Conflicts
		{shortName, false},
		{path.Join(shortName, "qux"), false},
		{path.Join(shortName, "quux"), false},
	}

	for _, tc := range cases {
		nativeName := filepath.FromSlash(tc.name)
		if res := osutil.CheckNameConflict("testdata", nativeName); res != tc.conflictFree {
			t.Errorf("CheckNameConflict(%q) = %v, should be %v", tc.name, res, tc.conflictFree)
		}
	}
}

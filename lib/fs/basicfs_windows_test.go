// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build windows
// +build windows

package fs

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/syncthing/syncthing/lib/build"
)

func TestWindowsPaths(t *testing.T) {
	testCases := []struct {
		input        string
		expectedRoot string
		expectedURI  string
	}{
		{`e:\`, `\\?\e:\`, `e:\`},
		{`\\?\e:\`, `\\?\e:\`, `e:\`},
		{`\\192.0.2.22\network\share`, `\\192.0.2.22\network\share`, `\\192.0.2.22\network\share`},
	}

	for _, testCase := range testCases {
		fs := newBasicFilesystem(testCase.input)
		if fs.root != testCase.expectedRoot {
			t.Errorf("root %q != %q", fs.root, testCase.expectedRoot)
		}
		if fs.URI() != testCase.expectedURI {
			t.Errorf("uri %q != %q", fs.URI(), testCase.expectedURI)
		}
	}

	fs := newBasicFilesystem(`relative\path`)
	if fs.root == `relative\path` || !strings.HasPrefix(fs.root, "\\\\?\\") {
		t.Errorf("%q == %q, expected absolutification", fs.root, `relative\path`)
	}
}

func TestResolveWindows83(t *testing.T) {
	fs, dir := setup(t)
	if isMaybeWin83(dir) {
		dir = fs.resolveWin83(dir)
		fs = newBasicFilesystem(dir)
	}

	shortAbs, _ := fs.rooted("LFDATA~1")
	long := "LFDataTool"
	longAbs, _ := fs.rooted(long)
	deleted, _ := fs.rooted(filepath.Join("foo", "LFDATA~1"))
	notShort, _ := fs.rooted(filepath.Join("foo", "bar", "baz"))

	fd, err := fs.Create(long)
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	if res := fs.resolveWin83(shortAbs); res != longAbs {
		t.Errorf(`Resolving for 8.3 names of "%v" resulted in "%v", expected "%v"`, shortAbs, res, longAbs)
	}
	if res := fs.resolveWin83(deleted); res != filepath.Dir(deleted) {
		t.Errorf(`Resolving for 8.3 names of "%v" resulted in "%v", expected "%v"`, deleted, res, filepath.Dir(deleted))
	}
	if res := fs.resolveWin83(notShort); res != notShort {
		t.Errorf(`Resolving for 8.3 names of "%v" resulted in "%v", expected "%v"`, notShort, res, notShort)
	}
}

func TestIsWindows83(t *testing.T) {
	fs, dir := setup(t)
	if isMaybeWin83(dir) {
		dir = fs.resolveWin83(dir)
		fs = newBasicFilesystem(dir)
	}

	tempTop, _ := fs.rooted(TempName("baz"))
	tempBelow, _ := fs.rooted(filepath.Join("foo", "bar", TempName("baz")))
	short, _ := fs.rooted(filepath.Join("LFDATA~1", TempName("baz")))
	tempAndShort, _ := fs.rooted(filepath.Join("LFDATA~1", TempName("baz")))

	for _, f := range []string{tempTop, tempBelow} {
		if isMaybeWin83(f) {
			t.Errorf(`"%v" is not a windows 8.3 path"`, f)
		}
	}

	for _, f := range []string{short, tempAndShort} {
		if !isMaybeWin83(f) {
			t.Errorf(`"%v" is not a windows 8.3 path"`, f)
		}
	}
}

func TestRelUnrootedCheckedWindows(t *testing.T) {
	testCases := []struct {
		root        string
		abs         string
		expectedRel string
	}{
		{`c:\`, `c:\foo`, `foo`},
		{`C:\`, `c:\foo`, `foo`},
		{`C:\`, `C:\foo`, `foo`},
		{`c:\`, `C:\foo`, `foo`},
		{`\\?c:\`, `\\?c:\foo`, `foo`},
		{`\\?C:\`, `\\?c:\foo`, `foo`},
		{`\\?C:\`, `\\?C:\foo`, `foo`},
		{`\\?c:\`, `\\?C:\foo`, `foo`},
		{`c:\foo`, `c:\foo\bar`, `bar`},
		{`c:\foo`, `c:\foo\bAr`, `bAr`},
		{`c:\foO`, `c:\Foo\bar`, `bar`},
		{`c:\foO`, `c:\fOo\bAr`, `bAr`},
		{`c:\foO`, `c:\fOo`, ``},
		{`C:\foO`, `c:\fOo`, ``},
	}

	for _, tc := range testCases {
		if res := rel(tc.abs, tc.root); res != tc.expectedRel {
			t.Errorf(`rel("%v", "%v") == "%v", expected "%v"`, tc.abs, tc.root, res, tc.expectedRel)
		}

		// unrootedChecked really just wraps rel, and does not care about
		// the actual root of that filesystem, but should not return an error
		// on these test cases.
		for _, root := range []string{tc.root, strings.ToLower(tc.root), strings.ToUpper(tc.root)} {
			fs := BasicFilesystem{root: root}
			if res, err := fs.unrootedChecked(tc.abs, []string{tc.root}); err != nil {
				t.Errorf(`Unexpected error from unrootedChecked("%v", "%v"): %v (fs.root: %v)`, tc.abs, tc.root, err, root)
			} else if res != tc.expectedRel {
				t.Errorf(`unrootedChecked("%v", "%v") == "%v", expected "%v" (fs.root: %v)`, tc.abs, tc.root, res, tc.expectedRel, root)
			}
		}
	}
}

// TestMultipleRoot checks that fs.unrootedChecked returns the correct path
// when given more than one possible root path.
func TestMultipleRoot(t *testing.T) {
	root := `c:\foO`
	roots := []string{root, `d:\`}
	rel := `bar`
	path := filepath.Join(root, rel)
	fs := BasicFilesystem{root: root}
	if res, err := fs.unrootedChecked(path, roots); err != nil {
		t.Errorf(`Unexpected error from unrootedChecked("%v", "%v"): %v (fs.root: %v)`, path, roots, err, root)
	} else if res != rel {
		t.Errorf(`unrootedChecked("%v", "%v") == "%v", expected "%v" (fs.root: %v)`, path, roots, res, rel, root)
	}
}

func TestGetFinalPath(t *testing.T) {
	testCases := []struct {
		input         string
		expectedPath  string
		eqToEvalSyml  bool
		ignoreMissing bool
	}{
		{`c:\`, `C:\`, true, false},
		{`\\?\c:\`, `C:\`, false, false},
		{`c:\wInDows\sYstEm32`, `C:\Windows\System32`, true, false},
		{`c:\parent\child`, `C:\parent\child`, false, true},
	}

	for _, testCase := range testCases {
		out, err := getFinalPathName(testCase.input)
		if err != nil {
			if testCase.ignoreMissing && os.IsNotExist(err) {
				continue
			}
			t.Errorf("getFinalPathName failed at %q with error %s", testCase.input, err)
		}
		// Trim UNC prefix
		if strings.HasPrefix(out, `\\?\UNC\`) {
			out = `\` + out[7:]
		} else {
			out = strings.TrimPrefix(out, `\\?\`)
		}
		if out != testCase.expectedPath {
			t.Errorf("getFinalPathName got wrong path: %q (expected %q)", out, testCase.expectedPath)
		}
		if testCase.eqToEvalSyml {
			evlPath, err1 := filepath.EvalSymlinks(testCase.input)
			if err1 != nil || out != evlPath {
				t.Errorf("EvalSymlinks got different results %q %s", evlPath, err1)
			}
		}
	}
}

func TestRemoveWindowsDirIcon(t *testing.T) {
	//Try to delete a folder with a custom icon with os.Remove (simulated by the readonly file attribute)

	fs, dir := setup(t)
	relativePath := "folder_with_icon"
	path := filepath.Join(dir, relativePath)

	if err := os.Mkdir(path, os.ModeDir); err != nil {
		t.Fatal(err)
	}
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(e)
	}
	if err := syscall.SetFileAttributes(ptr, uint32(syscall.FILE_ATTRIBUTE_DIRECTORY+syscall.FILE_ATTRIBUTE_READONLY)); err != nil {
		t.Fatal(err)
	}
	if err := fs.Remove(relativePath); err != nil {
		t.Fatal(err)
	}
}

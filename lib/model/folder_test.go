// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/d4l3k/messagediff"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/fs"
)

type unifySubsCase struct {
	in     []string // input to unifySubs
	exists []string // paths that exist in the database
	out    []string // expected output
}

func unifySubsCases() []unifySubsCase {
	cases := []unifySubsCase{
		{
			// 0. trailing slashes are cleaned, known paths are just passed on
			[]string{"foo/", "bar//"},
			[]string{"foo", "bar"},
			[]string{"bar", "foo"}, // the output is sorted
		},
		{
			// 1. "foo/bar" gets trimmed as it's covered by foo
			[]string{"foo", "bar/", "foo/bar/"},
			[]string{"foo", "bar"},
			[]string{"bar", "foo"},
		},
		{
			// 2. "" gets simplified to the empty list; ie scan all
			[]string{"foo", ""},
			[]string{"foo"},
			nil,
		},
		{
			// 3. "foo/bar" is unknown, but it's kept
			// because its parent is known
			[]string{"foo/bar"},
			[]string{"foo"},
			[]string{"foo/bar"},
		},
		{
			// 4. two independent known paths, both are kept
			// "usr/lib" is not a prefix of "usr/libexec"
			[]string{"usr/lib", "usr/libexec"},
			[]string{"usr", "usr/lib", "usr/libexec"},
			[]string{"usr/lib", "usr/libexec"},
		},
		{
			// 5. "usr/lib" is a prefix of "usr/lib/exec"
			[]string{"usr/lib", "usr/lib/exec"},
			[]string{"usr", "usr/lib", "usr/libexec"},
			[]string{"usr/lib"},
		},
		{
			// 6. .stignore and .stfolder are special and are passed on
			// verbatim even though they are unknown
			[]string{config.DefaultMarkerName, ".stignore"},
			[]string{},
			[]string{config.DefaultMarkerName, ".stignore"},
		},
		{
			// 7. but the presence of something else unknown forces an actual
			// scan
			[]string{config.DefaultMarkerName, ".stignore", "foo/bar"},
			[]string{},
			[]string{config.DefaultMarkerName, ".stignore", "foo"},
		},
		{
			// 8. explicit request to scan all
			nil,
			[]string{"foo"},
			nil,
		},
		{
			// 9. empty list of subs
			[]string{},
			[]string{"foo"},
			nil,
		},
		{
			// 10. absolute path
			[]string{"/foo"},
			[]string{"foo"},
			[]string{"foo"},
		},
	}

	if runtime.GOOS == "windows" {
		// Fixup path separators
		for i := range cases {
			for j, p := range cases[i].in {
				cases[i].in[j] = filepath.FromSlash(p)
			}
			for j, p := range cases[i].exists {
				cases[i].exists[j] = filepath.FromSlash(p)
			}
			for j, p := range cases[i].out {
				cases[i].out[j] = filepath.FromSlash(p)
			}
		}
	}

	return cases
}

func unifyExists(f string, tc unifySubsCase) bool {
	for _, e := range tc.exists {
		if f == e {
			return true
		}
	}
	return false
}

func TestUnifySubs(t *testing.T) {
	cases := unifySubsCases()
	for i, tc := range cases {
		exists := func(f string) bool {
			return unifyExists(f, tc)
		}
		out := unifySubs(tc.in, exists)
		if diff, equal := messagediff.PrettyDiff(tc.out, out); !equal {
			t.Errorf("Case %d failed; got %v, expected %v, diff:\n%s", i, out, tc.out, diff)
		}
	}
}

func BenchmarkUnifySubs(b *testing.B) {
	cases := unifySubsCases()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tc := range cases {
			exists := func(f string) bool {
				return unifyExists(f, tc)
			}
			unifySubs(tc.in, exists)
		}
	}
}

func TestIsDeleted(t *testing.T) {
	type tc struct {
		path  string
		isDel bool
	}
	cases := []tc{
		{"del", true},
		{"del.file", false},
		{filepath.Join("del", "del"), true},
		{"file", false},
		{"linkToFile", false},
		{"linkToDel", false},
		{"linkToDir", false},
		{filepath.Join("linkToDir", "file"), true},
		{filepath.Join("file", "behindFile"), true},
		{"dir", false},
		{"dir.file", false},
		{filepath.Join("dir", "file"), false},
		{filepath.Join("dir", "del"), true},
		{filepath.Join("dir", "del", "del"), true},
		{filepath.Join("del", "del", "del"), true},
	}

	tfcfg := testFolderConfigTmp()
	testFs := tfcfg.Filesystem()

	defer func() {
		testFs.Chmod("inacc", 0777)
		os.RemoveAll(testFs.URI())
	}()

	testFs.MkdirAll("dir", 0777)
	for _, f := range []string{"file", "del.file", "dir.file", filepath.Join("dir", "file")} {
		fd, err := testFs.Create(f)
		if err != nil {
			t.Fatal(err)
		}
		fd.Close()
	}
	if runtime.GOOS != "windows" {
		// Can't create unreadable dir on windows
		testFs.MkdirAll("inacc", 0777)
		if err := testFs.Chmod("inacc", 0000); err == nil {
			if _, err := testFs.Lstat(filepath.Join("inacc", "file")); fs.IsPermission(err) {
				// May fail e.g. if tests are run as root -> just skip
				cases = append(cases, tc{"inacc", false}, tc{filepath.Join("inacc", "file"), false})
			}
		}
	}
	for _, n := range []string{"Dir", "File", "Del"} {
		if err := fs.DebugSymlinkForTestsOnly(testFs, testFs, strings.ToLower(n), "linkTo"+n); err != nil {
			if runtime.GOOS == "windows" {
				t.Skip("Symlinks aren't working")
			}
			t.Fatal(err)
		}
		l.Infoln("weird thingS", filepath.Join(testFs.URI(), strings.ToLower(n)), filepath.Join(testFs.URI(), "linkTo"+n))
	}

	f := &folder{FolderConfiguration: tfcfg}
	for _, c := range cases {
		if del, err := f.isDeleted(testFs, c.path); err != nil {
			t.Errorf("IsDeleted(%v) returned error %v", c.path, err)
		} else if del != c.isDel {
			t.Errorf("IsDeleted(%v) != %v", c.path, c.isDel)
		}
	}
}

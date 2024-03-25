// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"errors"
	"fmt"
	osexec "os/exec"
	"path/filepath"
	"testing"

	"github.com/syncthing/syncthing/lib/build"
)

func testWalkSkipSymlink(t *testing.T, fsType FilesystemType, uri string) {
	if build.IsWindows {
		t.Skip("Symlinks skipping is not tested on windows")
	}

	fs := NewFilesystem(fsType, uri, testOpts...)

	if err := fs.MkdirAll("target/foo", 0755); err != nil {
		t.Fatal(err)
	}
	if err := fs.Mkdir("towalk", 0755); err != nil {
		t.Fatal(err)
	}
	if err := fs.CreateSymlink("target", "towalk/symlink"); err != nil {
		t.Fatal(err)
	}
	if err := fs.Walk("towalk", func(path string, info FileInfo, err error) error {
		if err != nil {
			t.Fatal(err)
		}
		if info.Name() != "symlink" && info.Name() != "towalk" {
			t.Fatal("Walk unexpected file", info.Name())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func createDirJunct(target string, name string) error {
	output, err := osexec.Command("cmd", "/c", "mklink", "/J", name, target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Failed to run mklink %v %v: %v %q", name, target, err, output)
	}
	return nil
}

func checkOptionJunctionsAsDirs(t *testing.T) bool {
	optionJunctionsAsDirs := false
	for _, opt := range testOpts {
		if _, ok := opt.(*OptionJunctionsAsDirs); ok {
			optionJunctionsAsDirs = true
			break
		}
	}
	if !optionJunctionsAsDirs {
		t.Skip("Only testing when OptionJunctionsAsDirs is set")
	}

	return optionJunctionsAsDirs
}

func testWalkTraverseDirJunct(t *testing.T, fsType FilesystemType, uri string) {
	if !build.IsWindows {
		t.Skip("Directory junctions are available and tested on windows only")
	}

	if !checkOptionJunctionsAsDirs(t) {
		return
	}

	fs := NewFilesystem(fsType, uri, testOpts...)

	if err := fs.MkdirAll("target/foo", 0); err != nil {
		t.Fatal(err)
	}
	if err := fs.Mkdir("towalk", 0); err != nil {
		t.Fatal(err)
	}
	if err := createDirJunct(filepath.Join(uri, "target"), filepath.Join(uri, "towalk/dirjunct")); err != nil {
		t.Fatal(err)
	}
	traversed := false
	if err := fs.Walk("towalk", func(path string, info FileInfo, err error) error {
		if err != nil {
			t.Fatal(err)
		}
		if info.Name() == "foo" {
			traversed = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !traversed {
		t.Fatal("Directory junction was not traversed")
	}
}

func testWalkInfiniteRecursion(t *testing.T, fsType FilesystemType, uri string) {
	if !build.IsWindows {
		t.Skip("Infinite recursion detection is tested on windows only")
	}

	if !checkOptionJunctionsAsDirs(t) {
		return
	}

	fs := NewFilesystem(fsType, uri, testOpts...)

	if err := fs.MkdirAll("target/foo", 0); err != nil {
		t.Fatal(err)
	}
	if err := fs.Mkdir("towalk", 0); err != nil {
		t.Fatal(err)
	}
	if err := createDirJunct(filepath.Join(uri, "target"), filepath.Join(uri, "towalk/dirjunct")); err != nil {
		t.Fatal(err)
	}
	if err := createDirJunct(filepath.Join(uri, "towalk"), filepath.Join(uri, "target/foo/recurse")); err != nil {
		t.Fatal(err)
	}
	dirjunctCnt := 0
	fooCnt := 0
	found := false
	if err := fs.Walk("towalk", func(path string, info FileInfo, err error) error {
		if err != nil {
			if errors.Is(err, ErrInfiniteRecursion) {
				if found {
					t.Fatal("second infinite recursion detected at", path)
				}
				found = true
				return nil
			}
			t.Fatal(err)
		}
		if info.Name() == "dirjunct" {
			dirjunctCnt++
		} else if info.Name() == "foo" {
			fooCnt++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if dirjunctCnt != 2 || fooCnt != 1 || !found {
		t.Fatal("Infinite recursion not detected correctly")
	}
}

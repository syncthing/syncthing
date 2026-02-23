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

	fs := NewFilesystem(fsType, uri)

	if err := fs.MkdirAll("target/foo", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fs.Mkdir("towalk", 0o755); err != nil {
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

func testWalkTraverseDirJunct(t *testing.T, fsType FilesystemType, uri string) {
	if !build.IsWindows {
		t.Skip("Directory junctions are available and tested on windows only")
	}

	fs := NewFilesystem(fsType, uri, new(OptionJunctionsAsDirs))

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

	fs := NewFilesystem(fsType, uri, new(OptionJunctionsAsDirs))

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

// TestWalkLexOrder verifies that the walk produces lexicographic order
// matching DB ORDER BY name exactly.
func TestWalkLexOrder(t *testing.T) {
	fs := NewFilesystem(FilesystemTypeFake, "TestWalkLexOrder")

	if err := fs.MkdirAll("a", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fs.Mkdir("b", 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "a/z.txt", "b.txt", "b/y.txt"} {
		dir := filepath.Dir(name)
		if dir != "." {
			fs.MkdirAll(dir, 0o755)
		}
		f, err := fs.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}

	var visited []string
	err := fs.Walk(".", func(path string, info FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path != "." {
			visited = append(visited, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Expected order matches: SELECT name FROM files ORDER BY name
	expected := []string{".stfolder", "a", "a.txt", "a/z.txt", "b", "b.txt", "b/y.txt"}

	if len(visited) != len(expected) {
		t.Fatalf("Wrong number of entries: got %v, expected %v", visited, expected)
	}
	for i, path := range visited {
		if path != expected[i] {
			t.Fatalf("Wrong order at index %d: got %q, expected %q.\nFull order: %v", i, path, expected[i], visited)
		}
	}
}

// TestWalkLexOrderImsodinExample tests the exact example from imsodin's review
// that exposed the ordering bug. This ensures walk order matches DB ORDER BY name
// for complex directory structures with similar prefixes.
//
// Files: a.txt, a_a, a/aaa, a/bbb, a.d/aaa, a_a/aaa, b/aaa
// DB ORDER BY name: a, a.d, a.d/aaa, a.txt, a/aaa, a/bbb, a_a, a_a/aaa, b, b/aaa
func TestWalkLexOrderImsodinExample(t *testing.T) {
	fs := NewFilesystem(FilesystemTypeFake, "TestWalkImsodin")

	// Create imsodin's example structure
	dirs := []string{"a", "a.d", "a_a", "b"}
	for _, d := range dirs {
		if err := fs.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	files := []string{"a.txt", "a/aaa", "a/bbb", "a.d/aaa", "a_a/aaa", "b/aaa"}
	for _, name := range files {
		f, err := fs.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}

	var visited []string
	err := fs.Walk(".", func(path string, info FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path != "." {
			visited = append(visited, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Expected order matches: SELECT name FROM files ORDER BY name
	// Verified with SQLite:
	//   CREATE TABLE files (name TEXT);
	//   INSERT INTO files VALUES ('a'),('a.d'),('a.d/aaa'),('a.txt'),('a/aaa'),('a/bbb'),('a_a'),('a_a/aaa'),('b'),('b/aaa');
	//   SELECT name FROM files ORDER BY name;
	expected := []string{
		".stfolder", // auto-created by fake fs
		"a",
		"a.d",
		"a.d/aaa",
		"a.txt",
		"a/aaa",
		"a/bbb",
		"a_a",
		"a_a/aaa",
		"b",
		"b/aaa",
	}

	if len(visited) != len(expected) {
		t.Fatalf("Wrong count: got %d, expected %d\ngot:      %v\nexpected: %v", len(visited), len(expected), visited, expected)
	}
	for i, path := range visited {
		if path != expected[i] {
			t.Fatalf("Wrong order at index %d: got %q, expected %q.\nFull order: %v", i, path, expected[i], visited)
		}
	}
}

// Copyright (c) 2014-2015 The Notify Authors. All rights reserved.
// Use of this source code is governed by the MIT license that can be
// found in the LICENSE file.

// +build !windows

package notify

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func tmpfile(s string) (string, error) {
	f, err := ioutil.TempFile(filepath.Split(s))
	if err != nil {
		return "", err
	}
	if err = nonil(f.Sync(), f.Close()); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func symlink(src, dst string) (string, error) {
	name, err := tmpfile(dst)
	if err != nil {
		return "", err
	}
	if err = nonil(os.Remove(name), os.Symlink(src, name)); err != nil {
		return "", err
	}
	return name, nil
}

func removeall(s ...string) {
	for _, s := range s {
		os.Remove(s)
	}
}

func TestCanonical(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd()=%v", err)
	}
	wdsym, err := symlink(wd, "")
	if err != nil {
		t.Fatalf(`symlink(%q, "")=%v`, wd, err)
	}
	td := filepath.Join(wd, "testdata")
	tdsym, err := symlink(td, td)
	if err != nil {
		t.Errorf("symlink(%q, %q)=%v", td, td, nonil(err, os.Remove(wdsym)))
	}
	defer removeall(wdsym, tdsym)
	vfstxt := filepath.Join(td, "vfs.txt")
	cases := [...]caseCanonical{
		{wdsym, wd},
		{tdsym, td},
		{filepath.Join(wdsym, "notify.go"), filepath.Join(wd, "notify.go")},
		{filepath.Join(tdsym, "vfs.txt"), vfstxt},
		{filepath.Join(wdsym, filepath.Base(tdsym), "vfs.txt"), vfstxt},
	}
	testCanonical(t, cases[:])
}

func TestCanonicalCircular(t *testing.T) {
	tmp1, err := tmpfile("circular")
	if err != nil {
		t.Fatal(err)
	}
	tmp2, err := tmpfile("circular")
	if err != nil {
		t.Fatal(nonil(err, os.Remove(tmp1)))
	}
	defer removeall(tmp1, tmp2)
	// Symlink tmp1 -> tmp2.
	if err = nonil(os.Remove(tmp1), os.Symlink(tmp2, tmp1)); err != nil {
		t.Fatal(err)
	}
	// Symlnik tmp2 -> tmp1.
	if err = nonil(os.Remove(tmp2), os.Symlink(tmp1, tmp2)); err != nil {
		t.Fatal(err)
	}
	if _, err = canonical(tmp1); err == nil {
		t.Fatalf("want canonical(%q)!=nil", tmp1)
	}
	if _, ok := err.(*os.PathError); !ok {
		t.Fatalf("want canonical(%q)=os.PathError; got %T", tmp1, err)
	}
}

// issue #83
func TestCanonical_RelativeSymlink(t *testing.T) {
	dir, err := ioutil.TempDir(wd, "")
	if err != nil {
		t.Fatalf("TempDir()=%v", err)
	}
	var (
		path     = filepath.Join(dir, filepath.FromSlash("a/b/c/d/e/f"))
		realpath = filepath.Join(dir, filepath.FromSlash("a/b/x/y/z/d/e/f"))
		rel      = filepath.FromSlash("x/y/z/../z/../z")
		chdir    = filepath.Join(dir, filepath.FromSlash("a/b"))
	)
	defer os.RemoveAll(dir)
	if err = os.MkdirAll(realpath, 0755); err != nil {
		t.Fatalf("MkdirAll()=%v", err)
	}
	if err := os.Chdir(chdir); err != nil {
		t.Fatalf("Chdir()=%v", err)
	}
	if err := nonil(os.Symlink(rel, "c"), os.Chdir(wd)); err != nil {
		t.Fatalf("Symlink()=%v", err)
	}
	got, err := canonical(path)
	if err != nil {
		t.Fatalf("canonical(%s)=%v", path, err)
	}
	if got != realpath {
		t.Fatalf("want canonical()=%s; got %s", realpath, got)
	}
}

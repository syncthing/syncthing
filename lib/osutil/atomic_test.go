// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateAtomicCreate(t *testing.T) {
	os.RemoveAll("testdata")
	defer os.RemoveAll("testdata")

	if err := os.Mkdir("testdata", 0755); err != nil {
		t.Fatal(err)
	}

	w, err := CreateAtomic("testdata/file")
	if err != nil {
		t.Fatal(err)
	}

	n, err := w.Write([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatal("written bytes", n, "!= 5")
	}

	if _, err := os.ReadFile("testdata/file"); err == nil {
		t.Fatal("file should not exist")
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	bs, err := os.ReadFile("testdata/file")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bs, []byte("hello")) {
		t.Error("incorrect data")
	}
}

func TestCreateAtomicReplace(t *testing.T) {
	testCreateAtomicReplace(t, 0666)
}
func TestCreateAtomicReplaceReadOnly(t *testing.T) {
	testCreateAtomicReplace(t, 0444) // windows compatible read-only bits
}

func testCreateAtomicReplace(t *testing.T, oldPerms os.FileMode) {
	t.Helper()

	testdir, err := os.MkdirTemp("", "syncthing")
	if err != nil {
		t.Fatal(err)
	}
	testfile := filepath.Join(testdir, "testfile")

	os.RemoveAll(testdir)
	defer os.RemoveAll(testdir)

	if err := os.Mkdir(testdir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(testfile, []byte("some old data"), oldPerms); err != nil {
		t.Fatal(err)
	}

	// Go < 1.14 has a bug in WriteFile where it does not use the requested
	// permissions on Windows. Chmod to make sure.
	if err := os.Chmod(testfile, oldPerms); err != nil {
		t.Fatal(err)
	}
	// Trust, but verify.
	if info, err := os.Stat(testfile); err != nil {
		t.Fatal(err)
	} else if info.Mode() != oldPerms {
		t.Fatalf("Wrong perms 0%o", info.Mode())
	}

	w, err := CreateAtomic(testfile)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	bs, err := os.ReadFile(testfile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bs, []byte("hello")) {
		t.Error("incorrect data")
	}

	if info, err := os.Stat(testfile); err != nil {
		t.Fatal(err)
	} else if info.Mode() != oldPerms {
		t.Fatalf("Perms changed during atomic write: 0%o", info.Mode())
	}
}

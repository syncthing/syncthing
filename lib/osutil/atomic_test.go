// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package osutil

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"
)

func TestCreateAtomicCreate(t *testing.T) {
	os.RemoveAll("testdata")
	defer os.RemoveAll("testdata")

	if err := os.Mkdir("testdata", 0755); err != nil {
		t.Fatal(err)
	}

	w, err := CreateAtomic("testdata/file", 0644)
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

	if _, err := ioutil.ReadFile("testdata/file"); err == nil {
		t.Fatal("file should not exist")
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	bs, err := ioutil.ReadFile("testdata/file")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bs, []byte("hello")) {
		t.Error("incorrect data")
	}
}

func TestCreateAtomicReplace(t *testing.T) {
	os.RemoveAll("testdata")
	defer os.RemoveAll("testdata")

	if err := os.Mkdir("testdata", 0755); err != nil {
		t.Fatal(err)
	}

	if err := ioutil.WriteFile("testdata/file", []byte("some old data"), 0644); err != nil {
		t.Fatal(err)
	}

	w, err := CreateAtomic("testdata/file", 0644)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	bs, err := ioutil.ReadFile("testdata/file")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bs, []byte("hello")) {
		t.Error("incorrect data")
	}
}

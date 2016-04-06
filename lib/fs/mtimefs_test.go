// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package fs

import (
	"errors"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func TestMtimeFS(t *testing.T) {
	os.RemoveAll("testdata")
	defer os.RemoveAll("testdata")
	os.Mkdir("testdata", 0777)

	// This Filesystem just returns errors on Chtimes, much like a well known
	// mobile operating system.
	underlyingFS := brokenFilesystem{DefaultFilesystem}

	// The MtimeFS wraps the broken filesystem and should provide reliable
	// mtime despite its brokenness.
	mtimeFS := NewMtimeFS(make(minimalByteStore))
	mtimeFS.Filesystem = underlyingFS // MtimeFS otherwise uses the DefaultFilesystem

	if err := ioutil.WriteFile("testdata/test", []byte("some data"), 0666); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat("testdata/test")
	if err != nil {
		t.Fatal(err)
	}

	actualMtime := info.ModTime()          // Probably ~time.Now()
	otherMtime := time.Unix(1234567890, 0) // Some other time

	// Set the mtime using the MtimeFS
	err = mtimeFS.Chtimes("testdata/test", otherMtime, otherMtime)
	if err != nil {
		t.Fatal(err)
	}

	// Get it back again and verify that it "took"
	info, err = mtimeFS.Lstat("testdata/test")
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(otherMtime) {
		t.Errorf("Lstat: returned mtime %v != set mtime %v", info.ModTime(), otherMtime)
	}

	// Also using regular Stat instead of Lstat
	info, err = mtimeFS.Stat("testdata/test")
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(otherMtime) {
		t.Errorf("Stat: returned mtime %v != set mtime %v", info.ModTime(), otherMtime)
	}

	// Get it again directly from disk and verify it was in fact not changed there
	info, err = os.Stat("testdata/test")
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(actualMtime) {
		t.Errorf("Actual mtime has changed; %v != %v", info.ModTime(), otherMtime)
	}

	// Update it on disk and verify that the MtimeFS now tells the truth
	otherMtime = otherMtime.Add(25 * time.Hour)
	err = os.Chtimes("testdata/test", otherMtime, otherMtime)
	if err != nil {
		t.Fatal(err)
	}
	info, err = mtimeFS.Lstat("testdata/test")
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(otherMtime) {
		t.Errorf("Returned mtime %v != on disk mtime %v", info.ModTime(), otherMtime)
	}
}

type brokenFilesystem struct {
	Filesystem
}

func (brokenFilesystem) Chtimes(path string, atime, mtime time.Time) error {
	return errors.New("cannot Chtimes")
}

type minimalByteStore map[string][]byte

func (m minimalByteStore) PutBytes(path string, bytes []byte) {
	m[path] = bytes
}

func (m minimalByteStore) Bytes(path string) ([]byte, bool) {
	bs, ok := m[path]
	return bs, ok
}

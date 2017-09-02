// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAutoClosedFile(t *testing.T) {
	os.RemoveAll("_autoclose")
	defer os.RemoveAll("_autoclose")
	os.Mkdir("_autoclose", 0755)
	file := filepath.FromSlash("_autoclose/tmp")
	data := []byte("hello, world\n")

	// An autoclosed file that closes very quickly
	ac := newAutoclosedFile(file, time.Millisecond, time.Millisecond)
	if _, err := ac.Write(data); err != nil {
		t.Fatal(err)
	}

	// Wait for it to close
	start := time.Now()
	for {
		time.Sleep(time.Millisecond)
		ac.mut.Lock()
		fd := ac.fd
		ac.mut.Unlock()
		if fd == nil {
			break
		}
		if time.Since(start) > time.Second {
			t.Fatal("File should have been closed after first write")
		}
	}

	// Write more data, which should be an append.
	if _, err := ac.Write(data); err != nil {
		t.Fatal(err)
	}

	// Close.
	if err := ac.Close(); err != nil {
		t.Fatal(err)
	}

	// The file should have both writes in it.
	bs, err := ioutil.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 2*len(data) {
		t.Fatalf("Writes failed, expected %d bytes, not %d", 2*len(data), len(bs))
	}

	// Open the file again.
	ac = newAutoclosedFile(file, time.Millisecond, time.Millisecond)

	// Write zero bytes. This should truncate it.
	if _, err := ac.Write(nil); err != nil {
		t.Fatal(err)
	}

	// It should now contain zero bytes.
	bs, err = ioutil.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 0 {
		t.Fatalf("Truncate failed, expected 0 bytes, not %d", len(bs))
	}

	// Write something
	if _, err := ac.Write(data); err != nil {
		t.Fatal(err)
	}

	// Close.
	if err := ac.Close(); err != nil {
		t.Fatal(err)
	}

	// It should now contain one write.
	bs, err = ioutil.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != len(data) {
		t.Fatalf("Write failed, expected %d bytes, not %d", len(data), len(bs))
	}
}

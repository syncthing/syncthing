// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build integration
// +build integration

package integration

import (
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIgnores(t *testing.T) {
	// Clean and start a syncthing instance

	log.Println("Cleaning...")
	err := removeAll("s1", "h1/index*")
	if err != nil {
		t.Fatal(err)
	}

	p := startInstance(t, 1)
	defer checkedStop(t, p)

	// Create eight empty files and directories

	dirs := []string{"d1", "d2", "d3", "d4", "d11", "d12", "d13", "d14"}
	files := []string{"f1", "f2", "f3", "f4", "f11", "f12", "f13", "f14", "d1/f1.TXT"}

	for _, dir := range dirs {
		err := os.Mkdir(filepath.Join("s1", dir), 0o755)
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, file := range files {
		fd, err := os.Create(filepath.Join("s1", file))
		if err != nil {
			t.Fatal(err)
		}
		fd.Close()
	}

	// Rescan and verify that we see them all

	if err := p.Rescan("default"); err != nil {
		t.Fatal(err)
	}

	m, err := p.Model("default")
	if err != nil {
		t.Fatal(err)
	}
	expected := len(files) // nothing is ignored
	if m.LocalFiles != expected {
		t.Fatalf("Incorrect number of files after initial scan, %d != %d", m.LocalFiles, expected)
	}

	// Add some of them to an ignore file

	err = os.WriteFile("s1/.stignore",
		[]byte("f1*\nf2\nd1*\nd2\ns1*\ns2\n(?i)*.txt"), // [fds][34] only non-ignored items
		0o644)
	if err != nil {
		t.Fatal(err)
	}

	// Rescan and verify that we see them

	if err := p.Rescan("default"); err != nil {
		t.Fatal(err)
	}

	m, err = p.Model("default")
	if err != nil {
		t.Fatal(err)
	}
	expected = len(files) * 2 / 8 // two out of eight items should remain
	if m.LocalFiles != expected {
		t.Fatalf("Incorrect number of files after first ignore, %d != %d", m.LocalFiles, expected)
	}

	// Change the pattern to include some of the files and dirs previously ignored

	time.Sleep(1100 * time.Millisecond)
	err = os.WriteFile("s1/.stignore", []byte("f2\nd2\ns2\n"), 0o644)

	// Rescan and verify that we see them

	if err := p.Rescan("default"); err != nil {
		t.Fatal(err)
	}

	m, err = p.Model("default")
	if err != nil {
		t.Fatal(err)
	}
	expected = len(files)*7/8 + 1 // seven out of eight items should remain, plus the foo.TXT
	if m.LocalFiles != expected {
		t.Fatalf("Incorrect number of files after second ignore, %d != %d", m.LocalFiles, expected)
	}
}

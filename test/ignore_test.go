// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build integration

package integration

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/syncthing/syncthing/lib/symlinks"
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

	files := []string{"f1", "f2", "f3", "f4", "f11", "f12", "f13", "f14"}
	dirs := []string{"d1", "d2", "d3", "d4", "d11", "d12", "d13", "d14"}
	all := append(files, dirs...)

	for _, file := range files {
		fd, err := os.Create(filepath.Join("s1", file))
		if err != nil {
			t.Fatal(err)
		}
		fd.Close()
	}

	for _, dir := range dirs {
		err := os.Mkdir(filepath.Join("s1", dir), 0755)
		if err != nil {
			t.Fatal(err)
		}
	}

	var syms []string
	if symlinksSupported() {
		syms = []string{"s1", "s2", "s3", "s4", "s11", "s12", "s13", "s14"}
		for _, sym := range syms {
			p := filepath.Join("s1", sym)
			symlinks.Create(p, p, 0)
		}
		all = append(all, syms...)
	}

	// Rescan and verify that we see them all

	if err := p.Rescan("default"); err != nil {
		t.Fatal(err)
	}

	m, err := p.Model("default")
	if err != nil {
		t.Fatal(err)
	}
	expected := len(all) // nothing is ignored
	if m.LocalFiles != expected {
		t.Fatalf("Incorrect number of files after initial scan, %d != %d", m.LocalFiles, expected)
	}

	// Add some of them to an ignore file

	fd, err := os.Create("s1/.stignore")
	if err != nil {
		t.Fatal(err)
	}
	_, err = fd.WriteString("f1*\nf2\nd1*\nd2\ns1*\ns2") // [fds][34] only non-ignored items
	if err != nil {
		t.Fatal(err)
	}
	err = fd.Close()
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
	expected = len(all) * 2 / 8 // two out of eight items of each type should remain
	if m.LocalFiles != expected {
		t.Fatalf("Incorrect number of files after first ignore, %d != %d", m.LocalFiles, expected)
	}

	// Change the pattern to include some of the files and dirs previously ignored

	fd, err = os.Create("s1/.stignore")
	if err != nil {
		t.Fatal(err)
	}
	_, err = fd.WriteString("f2\nd2\ns2\n")
	if err != nil {
		t.Fatal(err)
	}
	err = fd.Close()
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
	expected = len(all) * 7 / 8 // seven out of eight items of each type should remain
	if m.LocalFiles != expected {
		t.Fatalf("Incorrect number of files after second ignore, %d != %d", m.LocalFiles, expected)
	}
}

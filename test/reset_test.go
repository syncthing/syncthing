// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build integration
// +build integration

package integration

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReset(t *testing.T) {
	// Clean and start a syncthing instance

	log.Println("Cleaning...")
	err := removeAll("s1", "h1/index*")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("s1", 0o755); err != nil {
		t.Fatal(err)
	}

	log.Println("Creating files...")
	size := createFiles(t)

	p := startInstance(t, 1)
	defer p.Stop() // Not checkedStop, because Syncthing will exit on its own

	m, err := p.Model("default")
	if err != nil {
		t.Fatal(err)
	}
	expected := size
	if m.LocalFiles != expected {
		t.Fatalf("Incorrect number of files after initial scan, %d != %d", m.LocalFiles, expected)
	}

	// Clear all files but restore the folder marker
	log.Println("Cleaning...")
	err = removeAll("s1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("s1", 0o755); err != nil {
		t.Fatal(err)
	}
	if fd, err := os.Create("s1/.stfolder"); err != nil {
		t.Fatal(err)
	} else {
		fd.Close()
	}

	// Reset indexes of an invalid folder
	log.Println("Reset invalid folder")
	_, err = p.Post("/rest/system/reset?folder=invalid", nil)
	if err == nil {
		t.Fatalf("Cannot reset indexes of an invalid folder")
	}

	// Reset indexes of the default folder
	log.Println("Reset indexes of default folder")
	bs, err := p.Post("/rest/system/reset?folder=default", nil)
	if err != nil && err != io.ErrUnexpectedEOF {
		t.Fatalf("Failed to reset indexes (default): %v (%s)", err, bytes.TrimSpace(bs))
	}

	// ---- Syncthing exits here ----
	select {
	case <-p.Stopped():
	case <-time.After(20 * time.Second):
		t.Fatal("timed out before Syncthing stopped")
	}

	p = startInstance(t, 1)
	defer p.Stop() // Not checkedStop, because Syncthing will exit on its own

	m, err = p.Model("default")
	if err != nil {
		t.Fatal(err)
	}
	expected = 0
	if m.LocalFiles != expected {
		t.Fatalf("Incorrect number of files after initial scan, %d != %d", m.LocalFiles, expected)
	}

	// Recreate the files and scan
	log.Println("Creating files...")
	size = createFiles(t)

	if err := p.Rescan("default"); err != nil {
		t.Fatal(err)
	}

	// Verify that we see them
	m, err = p.Model("default")
	if err != nil {
		t.Fatal(err)
	}
	expected = size
	if m.LocalFiles != expected {
		t.Fatalf("Incorrect number of files after second creation phase, %d != %d", m.LocalFiles, expected)
	}

	// Reset all indexes
	log.Println("Reset DB...")
	bs, err = p.Post("/rest/system/reset", nil)
	if err != nil && err != io.ErrUnexpectedEOF {
		t.Fatalf("Failed to reset indexes (all): %v (%s)", err, bytes.TrimSpace(bs))
	}

	// ---- Syncthing exits here ----
	select {
	case <-p.Stopped():
	case <-time.After(20 * time.Second):
		t.Fatal("timed out before Syncthing stopped")
	}

	p = startInstance(t, 1)
	defer checkedStop(t, p)

	m, err = p.Model("default")
	if err != nil {
		t.Fatal(err)
	}
	expected = size
	if m.LocalFiles != expected {
		t.Fatalf("Incorrect number of files after initial scan, %d != %d", m.LocalFiles, expected)
	}
}

func createFiles(t *testing.T) int {
	// Create a few files

	const n = 8
	for i := 0; i < n; i++ {
		file := fmt.Sprintf("f%d", i)
		if err := os.WriteFile(filepath.Join("s1", file), []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return n
}

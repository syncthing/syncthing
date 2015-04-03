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
	"time"
)

func TestReset(t *testing.T) {
	// Clean and start a syncthing instance

	log.Println("Cleaning...")
	err := removeAll("s1", "h1/index*")
	if err != nil {
		t.Fatal(err)
	}

	p := syncthingProcess{ // id1
		instance: "1",
		argv:     []string{"-home", "h1"},
		port:     8081,
		apiKey:   apiKey,
	}
	err = p.start()
	if err != nil {
		t.Fatal(err)
	}
	defer p.stop()

	// Wait for one scan to succeed, or up to 20 seconds... This is to let
	// startup, UPnP etc complete and make sure that we've performed folder
	// error checking which creates the folder path if it's missing.
	log.Println("Starting...")
	waitForScan(t, &p)

	log.Println("Creating files...")
	size := createFiles(t)

	log.Println("Scanning files...")
	waitForScan(t, &p)

	m, err := p.model("default")
	if err != nil {
		t.Fatal(err)
	}
	expected := size
	if m.LocalFiles != expected {
		t.Fatalf("Incorrect number of files after initial scan, %d != %d", m.LocalFiles, expected)
	}

	// Clear all files but restore the folder marker
	log.Println("Cleaning...")
	err = removeAll("s1/*", "h1/index*")
	if err != nil {
		t.Fatal(err)
	}
	os.Create("s1/.stfolder")

	// Reset indexes of an invalid folder
	log.Println("Reset invalid folder")
	err = p.reset("invalid")
	if err == nil {
		t.Fatalf("Cannot reset indexes of an invalid folder")
	}
	m, err = p.model("default")
	if err != nil {
		t.Fatal(err)
	}
	expected = size
	if m.LocalFiles != expected {
		t.Fatalf("Incorrect number of files after initial scan, %d != %d", m.LocalFiles, expected)
	}

	// Reset indexes of the default folder
	log.Println("Reset indexes of default folder")
	err = p.reset("default")
	if err != nil {
		t.Fatal("Failed to reset indexes of the default folder:", err)
	}

	// Wait for ST and scan
	p.start()
	waitForScan(t, &p)

	// Verify that we see them
	m, err = p.model("default")
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
	waitForScan(t, &p)

	// Verify that we see them
	m, err = p.model("default")
	if err != nil {
		t.Fatal(err)
	}
	expected = size
	if m.LocalFiles != expected {
		t.Fatalf("Incorrect number of files after second creation phase, %d != %d", m.LocalFiles, expected)
	}

	// Reset all indexes
	log.Println("Reset DB...")
	err = p.reset("")
	if err != nil {
		t.Fatalf("Failed to reset indexes", err)
	}

	// Wait for ST and scan
	p.start()
	waitForScan(t, &p)

	m, err = p.model("default")
	if err != nil {
		t.Fatal(err)
	}
	expected = size
	if m.LocalFiles != expected {
		t.Fatalf("Incorrect number of files after initial scan, %d != %d", m.LocalFiles, expected)
	}
}

func waitForScan(t *testing.T, p *syncthingProcess) {
	// Wait for one scan to succeed, or up to 20 seconds...
	for i := 0; i < 20; i++ {
		err := p.rescan("default")
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		break
	}
}

func createFiles(t *testing.T) int {
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

	return len(all)
}

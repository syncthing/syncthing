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
	"testing"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rc"
)

func TestFileTypeChange(t *testing.T) {
	// Use no versioning
	id, _ := protocol.DeviceIDFromString(id2)
	cfg, _, _ := config.Load("h2/config.xml", id, events.NoopLogger)
	fld := cfg.Folders()["default"]
	fld.Versioning = config.VersioningConfiguration{}
	cfg.SetFolder(fld)
	os.Rename("h2/config.xml", "h2/config.xml.orig")
	defer os.Rename("h2/config.xml.orig", "h2/config.xml")
	cfg.Save()

	testFileTypeChange(t)
}

func TestFileTypeChangeSimpleVersioning(t *testing.T) {
	// Use simple versioning
	id, _ := protocol.DeviceIDFromString(id2)
	cfg, _, _ := config.Load("h2/config.xml", id, events.NoopLogger)
	fld := cfg.Folders()["default"]
	fld.Versioning = config.VersioningConfiguration{
		Type:   "simple",
		Params: map[string]string{"keep": "5"},
	}
	cfg.SetFolder(fld)
	os.Rename("h2/config.xml", "h2/config.xml.orig")
	defer os.Rename("h2/config.xml.orig", "h2/config.xml")
	cfg.Save()

	testFileTypeChange(t)
}

func TestFileTypeChangeStaggeredVersioning(t *testing.T) {
	// Use staggered versioning
	id, _ := protocol.DeviceIDFromString(id2)
	cfg, _, _ := config.Load("h2/config.xml", id, events.NoopLogger)
	fld := cfg.Folders()["default"]
	fld.Versioning = config.VersioningConfiguration{
		Type: "staggered",
	}
	cfg.SetFolder(fld)
	os.Rename("h2/config.xml", "h2/config.xml.orig")
	defer os.Rename("h2/config.xml.orig", "h2/config.xml")
	cfg.Save()

	testFileTypeChange(t)
}

func testFileTypeChange(t *testing.T) {
	log.Println("Cleaning...")
	err := removeAll("s1", "s2", "h1/index*", "h2/index*")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generating files...")
	err = generateFiles("s1", 100, 20, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}

	// A file that we will replace with a directory later

	if fd, err := os.Create("s1/fileToReplace"); err != nil {
		t.Fatal(err)
	} else {
		fd.Close()
	}

	// A directory that we will replace with a file later

	err = os.Mkdir("s1/emptyDirToReplace", 0755)
	if err != nil {
		t.Fatal(err)
	}

	// A directory with files that we will replace with a file later

	err = os.Mkdir("s1/dirToReplace", 0755)
	if err != nil {
		t.Fatal(err)
	}
	if fd, err := os.Create("s1/dirToReplace/emptyFile"); err != nil {
		t.Fatal(err)
	} else {
		fd.Close()
	}

	// Verify that the files and directories sync to the other side

	sender := startInstance(t, 1)
	defer checkedStop(t, sender)

	receiver := startInstance(t, 2)
	defer checkedStop(t, receiver)

	sender.ResumeAll()
	receiver.ResumeAll()

	log.Println("Syncing...")

	rc.AwaitSync("default", sender, receiver)

	// Delay scans for the moment
	if err := sender.RescanDelay("default", 86400); err != nil {
		t.Fatal(err)
	}

	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Making some changes...")

	// Replace file with directory

	err = os.RemoveAll("s1/fileToReplace")
	if err != nil {
		t.Fatal(err)
	}
	err = os.Mkdir("s1/fileToReplace", 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Replace empty directory with file

	err = os.RemoveAll("s1/emptyDirToReplace")
	if err != nil {
		t.Fatal(err)
	}
	if fd, err := os.Create("s1/emptyDirToReplace"); err != nil {
		t.Fatal(err)
	} else {
		fd.Close()
	}

	// Clear directory and replace with file

	err = os.RemoveAll("s1/dirToReplace")
	if err != nil {
		t.Fatal(err)
	}
	if fd, err := os.Create("s1/dirToReplace"); err != nil {
		t.Fatal(err)
	} else {
		fd.Close()
	}

	// Sync these changes and recheck

	log.Println("Syncing...")

	if err := sender.Rescan("default"); err != nil {
		t.Fatal(err)
	}

	rc.AwaitSync("default", sender, receiver)

	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}
}

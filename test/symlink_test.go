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

func TestSymlinks(t *testing.T) {
	if !symlinksSupported() {
		t.Skip("symlinks unsupported")
	}

	// Use no versioning
	id, _ := protocol.DeviceIDFromString(id2)
	cfg, _, _ := config.Load("h2/config.xml", id, events.NoopLogger)
	modifyConfig(t, cfg, func(c *config.Configuration) {
		fld, _, _ := c.Folder("default")
		fld.Versioning = config.VersioningConfiguration{}
		c.SetFolder(fld)
	})
	os.Rename("h2/config.xml", "h2/config.xml.orig")
	defer os.Rename("h2/config.xml.orig", "h2/config.xml")
	cfg.Save()

	testSymlinks(t)
}

func TestSymlinksSimpleVersioning(t *testing.T) {
	if !symlinksSupported() {
		t.Skip("symlinks unsupported")
	}

	// Use simple versioning
	id, _ := protocol.DeviceIDFromString(id2)
	cfg, _, _ := config.Load("h2/config.xml", id, events.NoopLogger)
	modifyConfig(t, cfg, func(c *config.Configuration) {
		fld, _, _ := c.Folder("default")
		fld.Versioning = config.VersioningConfiguration{
			Type:   "simple",
			Params: map[string]string{"keep": "5"},
		}
		c.SetFolder(fld)
	})
	os.Rename("h2/config.xml", "h2/config.xml.orig")
	defer os.Rename("h2/config.xml.orig", "h2/config.xml")
	cfg.Save()

	testSymlinks(t)
}

func TestSymlinksStaggeredVersioning(t *testing.T) {
	if !symlinksSupported() {
		t.Skip("symlinks unsupported")
	}

	// Use staggered versioning
	id, _ := protocol.DeviceIDFromString(id2)
	cfg, _, _ := config.Load("h2/config.xml", id, events.NoopLogger)
	modifyConfig(t, cfg, func(c *config.Configuration) {
		fld, _, _ := c.Folder("default")
		fld.Versioning = config.VersioningConfiguration{
			Type: "staggered",
		}
		c.SetFolder(fld)
	})
	os.Rename("h2/config.xml", "h2/config.xml.orig")
	defer os.Rename("h2/config.xml.orig", "h2/config.xml")
	cfg.Save()

	testSymlinks(t)
}

func testSymlinks(t *testing.T) {
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

	// A file that we will replace with a symlink later

	fd, err := os.Create("s1/fileToReplace")
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	// A directory that we will replace with a symlink later

	err = os.Mkdir("s1/dirToReplace", 0755)
	if err != nil {
		t.Fatal(err)
	}

	// A file and a symlink to that file

	fd, err = os.Create("s1/file")
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()
	err = os.Symlink("file", "s1/fileLink")
	if err != nil {
		log.Fatal(err)
	}

	// A directory and a symlink to that directory

	err = os.Mkdir("s1/dir", 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Symlink("dir", "s1/dirLink")
	if err != nil {
		log.Fatal(err)
	}

	// A link to something in the repo that does not exist

	err = os.Symlink("does/not/exist", "s1/noneLink")
	if err != nil {
		log.Fatal(err)
	}

	// A link we will replace with a file later

	err = os.Symlink("does/not/exist", "s1/repFileLink")
	if err != nil {
		log.Fatal(err)
	}

	// A link we will replace with a directory later

	err = os.Symlink("does/not/exist", "s1/repDirLink")
	if err != nil {
		log.Fatal(err)
	}

	// A link we will remove later

	err = os.Symlink("does/not/exist", "s1/removeLink")
	if err != nil {
		log.Fatal(err)
	}

	// Verify that the files and symlinks sync to the other side

	sender := startInstance(t, 1)
	defer checkedStop(t, sender)

	receiver := startInstance(t, 2)
	defer checkedStop(t, receiver)

	sender.ResumeAll()
	receiver.ResumeAll()

	log.Println("Syncing...")
	rc.AwaitSync("default", sender, receiver)

	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Making some changes...")

	// Remove one symlink

	err = os.Remove("s1/fileLink")
	if err != nil {
		log.Fatal(err)
	}

	// Change the target of another

	err = os.Remove("s1/dirLink")
	if err != nil {
		log.Fatal(err)
	}
	err = os.Symlink("file", "s1/dirLink")
	if err != nil {
		log.Fatal(err)
	}

	// Replace one with a file

	err = os.Remove("s1/repFileLink")
	if err != nil {
		log.Fatal(err)
	}

	fd, err = os.Create("s1/repFileLink")
	if err != nil {
		log.Fatal(err)
	}
	fd.Close()

	// Replace one with a directory

	err = os.Remove("s1/repDirLink")
	if err != nil {
		log.Fatal(err)
	}

	err = os.Mkdir("s1/repDirLink", 0755)
	if err != nil {
		log.Fatal(err)
	}

	// Replace a file with a symlink

	err = os.Remove("s1/fileToReplace")
	if err != nil {
		log.Fatal(err)
	}
	err = os.Symlink("somewhere/non/existent", "s1/fileToReplace")
	if err != nil {
		log.Fatal(err)
	}

	// Replace a directory with a symlink

	err = os.RemoveAll("s1/dirToReplace")
	if err != nil {
		log.Fatal(err)
	}
	err = os.Symlink("somewhere/non/existent", "s1/dirToReplace")
	if err != nil {
		log.Fatal(err)
	}

	// Remove a broken symlink

	err = os.Remove("s1/removeLink")
	if err != nil {
		log.Fatal(err)
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

// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build integration

package integration

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/rc"
	"github.com/syncthing/syncthing/lib/symlinks"
)

func symlinksSupported() bool {
	tmp, err := ioutil.TempDir("", "symlink-test")
	if err != nil {
		return false
	}
	defer os.RemoveAll(tmp)
	err = os.Symlink("tmp", filepath.Join(tmp, "link"))
	return err == nil
}

func TestSymlinks(t *testing.T) {
	if !symlinksSupported() {
		t.Skip("symlinks unsupported")
	}

	// Use no versioning
	id, _ := protocol.DeviceIDFromString(id2)
	cfg, _ := config.Load("h2/config.xml", id)
	fld := cfg.Folders()["default"]
	fld.Versioning = config.VersioningConfiguration{}
	cfg.SetFolder(fld)
	cfg.Save()

	testSymlinks(t)
}

func TestSymlinksSimpleVersioning(t *testing.T) {
	if !symlinksSupported() {
		t.Skip("symlinks unsupported")
	}

	// Use simple versioning
	id, _ := protocol.DeviceIDFromString(id2)
	cfg, _ := config.Load("h2/config.xml", id)
	fld := cfg.Folders()["default"]
	fld.Versioning = config.VersioningConfiguration{
		Type:   "simple",
		Params: map[string]string{"keep": "5"},
	}
	cfg.SetFolder(fld)
	cfg.Save()

	testSymlinks(t)
}

func TestSymlinksStaggeredVersioning(t *testing.T) {
	if !symlinksSupported() {
		t.Skip("symlinks unsupported")
	}

	// Use staggered versioning
	id, _ := protocol.DeviceIDFromString(id2)
	cfg, _ := config.Load("h2/config.xml", id)
	fld := cfg.Folders()["default"]
	fld.Versioning = config.VersioningConfiguration{
		Type: "staggered",
	}
	cfg.SetFolder(fld)
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
	err = symlinks.Create("s1/fileLink", "file", 0)
	if err != nil {
		log.Fatal(err)
	}

	// A directory and a symlink to that directory

	err = os.Mkdir("s1/dir", 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = symlinks.Create("s1/dirLink", "dir", 0)
	if err != nil {
		log.Fatal(err)
	}

	// A link to something in the repo that does not exist

	err = symlinks.Create("s1/noneLink", "does/not/exist", 0)
	if err != nil {
		log.Fatal(err)
	}

	// A link we will replace with a file later

	err = symlinks.Create("s1/repFileLink", "does/not/exist", 0)
	if err != nil {
		log.Fatal(err)
	}

	// A link we will replace with a directory later

	err = symlinks.Create("s1/repDirLink", "does/not/exist", 0)
	if err != nil {
		log.Fatal(err)
	}

	// A link we will remove later

	err = symlinks.Create("s1/removeLink", "does/not/exist", 0)
	if err != nil {
		log.Fatal(err)
	}

	// Verify that the files and symlinks sync to the other side

	sender := startInstance(t, 1)
	defer checkedStop(t, sender)

	receiver := startInstance(t, 2)
	defer checkedStop(t, receiver)

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
	err = symlinks.Create("s1/dirLink", "file", 0)
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
	err = symlinks.Create("s1/fileToReplace", "somewhere/non/existent", 0)
	if err != nil {
		log.Fatal(err)
	}

	// Replace a directory with a symlink

	err = os.RemoveAll("s1/dirToReplace")
	if err != nil {
		log.Fatal(err)
	}
	err = symlinks.Create("s1/dirToReplace", "somewhere/non/existent", 0)
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

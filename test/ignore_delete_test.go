// Copyright (C) 2015 The Syncthing Authors.
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
	"strings"
	"testing"
	"time"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/internal/config"
	"github.com/syncthing/syncthing/internal/rc"
)

func TestIgnoreDelete(t *testing.T) {
	// Enable "ignoreDelete" on s1/default
	id, _ := protocol.DeviceIDFromString(id1)
	cfg, _ := config.Load("h1/config.xml", id)
	fld := cfg.Folders()["default"]
	fld.IgnoreDelete = true
	cfg.SetFolder(fld)
	os.Rename("h1/config.xml", "h1/config.xml.orig")
	defer os.Rename("h1/config.xml.orig", "h1/config.xml")
	cfg.Save()

	log.Println("Cleaning...")
	err := removeAll("s1", "s2", "h1/index*", "h2/index*")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generating files...")
	err = os.Mkdir("s1", 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Mkdir("s2", 0755)
	if err != nil {
		t.Fatal(err)
	}

	fd, err := os.Create("s1/testfile.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = fd.WriteString("hello\n")
	if err != nil {
		t.Fatal(err)
	}
	err = fd.Close()
	if err != nil {
		t.Fatal(err)
	}

	expected, err := directoryContents("s1")
	if err != nil {
		t.Fatal(err)
	}

	master := startInstance(t, 1)
	defer checkedStop(t, master)

	remote := startInstance(t, 2)
	defer checkedStop(t, remote)

	log.Println("Syncing...")

	rc.AwaitSync("default", master, remote)

	log.Println("Verifying...")

	actual, err := directoryContents("s2")
	if err != nil {
		t.Fatal(err)
	}
	err = compareDirectoryContents(actual, expected)
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Deleting file on remote side...")

	err = os.Remove("s2/testfile.txt")
	if err != nil {
		t.Fatal(err)
	}

	if err := remote.Rescan("default"); err != nil {
		t.Fatal(err)
	}

	log.Println("Waiting for index to send...")

	time.Sleep(10 * time.Second)

	log.Println("Syncing...")

	rc.AwaitSync("default", master, remote)

	// Verify that the delete was ignored

	fd, err = os.Open("s1/testfile.txt")
	if err != nil {
		t.Fatal(err)
	}
	bs, err := ioutil.ReadAll(fd)
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	if strings.Contains(string(bs), "hello\n") {
		t.Error("Delete should not have been synced to master")
	}

	fd, err = os.Open("s2/testfile.txt")
	if err == nil {
		t.Error("File should have been deleted")
	}
}

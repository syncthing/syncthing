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
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/rc"
)

func TestOverride(t *testing.T) {
	// Enable "Master" on s1/default
	id, _ := protocol.DeviceIDFromString(id1)
	cfg, _ := config.Load("h1/config.xml", id)
	fld := cfg.Folders()["default"]
	fld.ReadOnly = true
	cfg.SetFolder(fld)
	os.Rename("h1/config.xml", "h1/config.xml.orig")
	defer osutil.Rename("h1/config.xml.orig", "h1/config.xml")
	cfg.Save()

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

	slave := startInstance(t, 2)
	defer checkedStop(t, slave)

	log.Println("Syncing...")

	rc.AwaitSync("default", master, slave)

	log.Println("Verifying...")

	actual, err := directoryContents("s2")
	if err != nil {
		t.Fatal(err)
	}
	err = compareDirectoryContents(actual, expected)
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Changing file on slave side...")

	fd, err = os.OpenFile("s2/testfile.txt", os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fd.WriteString("text added to s2\n")
	if err != nil {
		t.Fatal(err)
	}
	err = fd.Close()
	if err != nil {
		t.Fatal(err)
	}

	if err := slave.Rescan("default"); err != nil {
		t.Fatal(err)
	}

	log.Println("Waiting for index to send...")

	time.Sleep(10 * time.Second)

	log.Println("Hitting Override on master...")

	if _, err := master.Post("/rest/db/override?folder=default", nil); err != nil {
		t.Fatal(err)
	}

	log.Println("Syncing...")

	rc.AwaitSync("default", master, slave)

	// Verify that the override worked

	fd, err = os.Open("s1/testfile.txt")
	if err != nil {
		t.Fatal(err)
	}
	bs, err := ioutil.ReadAll(fd)
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	if strings.Contains(string(bs), "added to s2") {
		t.Error("Change should not have been synced to master")
	}

	fd, err = os.Open("s2/testfile.txt")
	if err != nil {
		t.Fatal(err)
	}
	bs, err = ioutil.ReadAll(fd)
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	if strings.Contains(string(bs), "added to s2") {
		t.Error("Change should have been overridden on slave")
	}
}

/* This doesn't currently work with detection completion, as we don't actually
get to completion when in master/slave mode. Needs fixing.

func TestOverrideIgnores(t *testing.T) {
	// Enable "Master" on s1/default
	id, _ := protocol.DeviceIDFromString(id1)
	cfg, _ := config.Load("h1/config.xml", id)
	fld := cfg.Folders()["default"]
	fld.ReadOnly = true
	cfg.SetFolder(fld)
	os.Rename("h1/config.xml", "h1/config.xml.orig")
	defer osutil.Rename("h1/config.xml.orig", "h1/config.xml")
	cfg.Save()

	log.Println("Cleaning...")
	err := removeAll("s1", "s2", "h1/index*", "h2/index*")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generating files...")
	err = generateFiles("s1", 10, 2, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}

	fd, err := os.Create("s1/testfile.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = fd.WriteString("original text\n")
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

	log.Println("Starting master...")
	master := syncthingProcess{ // id1
		instance: "1",
		argv:     []string{"-home", "h1"},
		port:     8081,
		apiKey:   apiKey,
	}
	err = master.start()
	if err != nil {
		t.Fatal(err)
	}
	defer master.stop()

	log.Println("Starting slave...")
	slave := syncthingProcess{ // id2
		instance: "2",
		argv:     []string{"-home", "h2"},
		port:     8082,
		apiKey:   apiKey,
	}
	err = slave.start()
	if err != nil {
		master.stop()
		t.Fatal(err)
	}
	defer slave.stop()

	log.Println("Syncing...")

	err = awaitCompletion("default", master, slave)
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Verifying...")

	actual, err := directoryContents("s2")
	if err != nil {
		t.Fatal(err)
	}
	err = compareDirectoryContents(actual, expected)
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Ignoring testfile.txt on master...")

	fd, err = os.Create("s1/.stignore")
	if err != nil {
		t.Fatal(err)
	}
	_, err = fd.WriteString("testfile.txt\n")
	if err != nil {
		t.Fatal(err)
	}
	err = fd.Close()
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Modify testfile.txt on master...")

	fd, err = os.Create("s1/testfile.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = fd.WriteString("updated on master but ignored\n")
	if err != nil {
		t.Fatal(err)
	}
	err = fd.Close()
	if err != nil {
		t.Fatal(err)
	}

	fd, err = os.Create("s1/testfile2.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = fd.WriteString("sync me\n")
	if err != nil {
		t.Fatal(err)
	}
	err = fd.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = master.rescan("default")

	log.Println("Waiting for sync...")
	time.Sleep(10 * time.Second)

	// Verify that sync worked

	fd, err = os.Open("s2/testfile.txt")
	if err != nil {
		t.Fatal(err)
	}
	bs, err := ioutil.ReadAll(fd)
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	if !strings.Contains(string(bs), "original text") {
		t.Error("Changes should not have been synced to slave")
	}

	fd, err = os.Open("s2/testfile2.txt")
	if err != nil {
		t.Fatal(err)
	}
	bs, err = ioutil.ReadAll(fd)
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	if !strings.Contains(string(bs), "sync me") {
		t.Error("Changes should have been synced to slave")
	}

	log.Println("Removing file on slave side...")

	os.Remove("s2/testfile.txt")

	err = slave.rescan("default")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Waiting for sync...")
	time.Sleep(10 * time.Second)

	// Verify that nothing changed

	fd, err = os.Open("s1/testfile.txt")
	if err != nil {
		t.Fatal(err)
	}
	bs, err = ioutil.ReadAll(fd)
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	if !strings.Contains(string(bs), "updated on master but ignored") {
		t.Error("Changes should not have been synced to master")
	}

	fd, err = os.Open("s2/testfile.txt")
	if err == nil {
		t.Error("File should not exist on the slave")
	}

	log.Println("Creating file on slave...")

	fd, err = os.Create("s2/testfile3.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = fd.WriteString("created on slave, should be removed on override\n")
	if err != nil {
		t.Fatal(err)
	}
	err = fd.Close()
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Hitting Override on master...")

	resp, err := master.post("/rest/db/override?folder=default", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatal(resp.Status)
	}

	log.Println("Waiting for sync...")
	time.Sleep(10 * time.Second)

	fd, err = os.Open("s2/testfile.txt")
	if err == nil {
		t.Error("File should not exist on the slave")
	}
	fd, err = os.Open("s2/testfile2.txt")
	if err != nil {
		t.Fatal(err)
	}
	bs, err = ioutil.ReadAll(fd)
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()
	if !strings.Contains(string(bs), "sync me") {
		t.Error("Changes should have been synced to slave")
	}
	fd, err = os.Open("s2/testfile3.txt")
	if err != nil {
		t.Error("File should still exist on the slave")
	}
	fd.Close()

	log.Println("Hitting Override on master (again)...")

	resp, err = master.post("/rest/db/override?folder=default", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatal(resp.Status)
	}

	log.Println("Waiting for sync...")
	time.Sleep(10 * time.Second)

	fd, err = os.Open("s2/testfile.txt")
	if err == nil {
		t.Error("File should not exist on the slave")
	}
	fd, err = os.Open("s2/testfile2.txt")
	if err != nil {
		t.Fatal(err)
	}
	bs, err = ioutil.ReadAll(fd)
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()
	if !strings.Contains(string(bs), "sync me") {
		t.Error("Changes should have been synced to slave")
	}
	fd, err = os.Open("s2/testfile3.txt")
	if err != nil {
		t.Error("File should still exist on the slave")
	}
	fd.Close()

}
*/

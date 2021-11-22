// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build integration
// +build integration

package integration

import (
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rc"
)

func TestOverride(t *testing.T) {
	// Enable "send-only" on s1/default
	id, _ := protocol.DeviceIDFromString(id1)
	cfg, _, _ := config.Load("h1/config.xml", id, events.NoopLogger)
	fld := cfg.Folders()["default"]
	fld.Type = config.FolderTypeSendOnly
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

	sendOnly := startInstance(t, 1)
	defer checkedStop(t, sendOnly)

	sendRecv := startInstance(t, 2)
	defer checkedStop(t, sendRecv)

	sendOnly.ResumeAll()
	sendRecv.ResumeAll()

	log.Println("Syncing...")

	rc.AwaitSync("default", sendOnly, sendRecv)

	log.Println("Verifying...")

	actual, err := directoryContents("s2")
	if err != nil {
		t.Fatal(err)
	}
	err = compareDirectoryContents(actual, expected)
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Changing file on sendRecv side...")

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

	if err := sendRecv.Rescan("default"); err != nil {
		t.Fatal(err)
	}

	log.Println("Waiting for index to send...")

	time.Sleep(10 * time.Second)

	log.Println("Hitting Override on sendOnly...")

	if _, err := sendOnly.Post("/rest/db/override?folder=default", nil); err != nil {
		t.Fatal(err)
	}

	log.Println("Syncing...")

	rc.AwaitSync("default", sendOnly, sendRecv)

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
		t.Error("Change should not have been synced to sendOnly")
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
		t.Error("Change should have been overridden on sendRecv")
	}
}

/* This doesn't currently work with detection completion, as we don't actually
get to completion when in sendOnly/sendRecv mode. Needs fixing.

func TestOverrideIgnores(t *testing.T) {
	// Enable "sendOnly" on s1/default
	id, _ := protocol.DeviceIDFromString(id1)
	cfg, _, _ := config.Load("h1/config.xml", id, events.NoopLogger)
	fld := cfg.Folders()["default"]
	fld.ReadOnly = true
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

	log.Println("Starting sendOnly...")
	sendOnly := syncthingProcess{ // id1
		instance: "1",
		argv:     []string{"--home", "h1"},
		port:     8081,
		apiKey:   apiKey,
	}
	err = sendOnly.start()
	if err != nil {
		t.Fatal(err)
	}
	defer sendOnly.stop()

	log.Println("Starting sendRecv...")
	sendRecv := syncthingProcess{ // id2
		instance: "2",
		argv:     []string{"--home", "h2"},
		port:     8082,
		apiKey:   apiKey,
	}
	err = sendRecv.start()
	if err != nil {
		sendOnly.stop()
		t.Fatal(err)
	}
	defer sendRecv.stop()

	log.Println("Syncing...")

	err = awaitCompletion("default", sendOnly, sendRecv)
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

	log.Println("Ignoring testfile.txt on sendOnly...")

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

	log.Println("Modify testfile.txt on sendOnly...")

	fd, err = os.Create("s1/testfile.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = fd.WriteString("updated on sendOnly but ignored\n")
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

	err = sendOnly.rescan("default")

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
		t.Error("Changes should not have been synced to sendRecv")
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
		t.Error("Changes should have been synced to sendRecv")
	}

	log.Println("Removing file on sendRecv side...")

	os.Remove("s2/testfile.txt")

	err = sendRecv.rescan("default")
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

	if !strings.Contains(string(bs), "updated on sendOnly but ignored") {
		t.Error("Changes should not have been synced to sendOnly")
	}

	fd, err = os.Open("s2/testfile.txt")
	if err == nil {
		t.Error("File should not exist on the sendRecv")
	}

	log.Println("Creating file on sendRecv...")

	fd, err = os.Create("s2/testfile3.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = fd.WriteString("created on sendRecv, should be removed on override\n")
	if err != nil {
		t.Fatal(err)
	}
	err = fd.Close()
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Hitting Override on sendOnly...")

	resp, err := sendOnly.post("/rest/db/override?folder=default", nil)
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
		t.Error("File should not exist on the sendRecv")
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
		t.Error("Changes should have been synced to sendRecv")
	}
	fd, err = os.Open("s2/testfile3.txt")
	if err != nil {
		t.Error("File should still exist on the sendRecv")
	}
	fd.Close()

	log.Println("Hitting Override on sendOnly (again)...")

	resp, err = sendOnly.post("/rest/db/override?folder=default", nil)
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
		t.Error("File should not exist on the sendRecv")
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
		t.Error("Changes should have been synced to sendRecv")
	}
	fd, err = os.Open("s2/testfile3.txt")
	if err != nil {
		t.Error("File should still exist on the sendRecv")
	}
	fd.Close()

}
*/

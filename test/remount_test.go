// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build integration

package integration

import (
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/syncthing/syncthing/internal/rc"
)

func TestRemountAfterSync(t *testing.T) {
	log.Println("Cleaning...")
	err := removeAll("s1", "s2", "h1/index*", "h2/index*", "s1-unmount")
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
	for i := 0; i < 1e6; i++ {
		_, err = fd.WriteString("hello\n")
		if err != nil {
			t.Fatal(err)
		}
	}
	err = fd.Close()
	if err != nil {
		t.Fatal(err)
	}

	expected, err := directoryContents("s1")
	if err != nil {
		t.Fatal(err)
	}

	sender := startInstance(t, 1)
	defer checkedStop(t, sender)
	receiver := startInstance(t, 2)
	defer checkedStop(t, receiver)

	// Rescan with a delay on the next one, so we are not surprised by a
	// sudden rescan while we're trying to 'unmount' the disk.

	if err := sender.RescanDelay("default", 86400); err != nil {
		t.Fatal(err)
	}
	if err := receiver.RescanDelay("default", 86400); err != nil {
		t.Fatal(err)
	}
	rc.AwaitSync("default", sender, receiver)

	log.Println("Verifying...")

	actual, err := directoryContents("s2")
	if err != nil {
		t.Fatal(err)
	}
	err = compareDirectoryContents(actual, expected)
	if err != nil {
		t.Fatal(err)
	}

	log.Println("'Unmounting' folder...")

	err = os.Rename("s1", "s1-unmount")
	defer os.Remove("s1-unmount")
	if err != nil {
		t.Fatal(err)
	}

	err = os.Remove("s2/testfile.txt")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Scan changes...")

	if err := sender.RescanDelay("default", 86400); err != nil {
		if !strings.Contains(err.Error(), "folder path missing") {
			t.Fatal(err)
		}
	}
	if err := receiver.RescanDelay("default", 86400); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)

	log.Println("'Remounting' folder...")

	err = os.Rename("s1-unmount", "s1")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Syncing...")

	if err := sender.RescanDelay("default", 86400); err != nil {
		t.Fatal(err)
	}
	if err := receiver.RescanDelay("default", 86400); err != nil {
		t.Fatal(err)
	}
	rc.AwaitSync("default", sender, receiver)

	fd, err = os.OpenFile("s1/testfile.txt", os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		t.Fatal("s1/testfile.txt should not exist")
	}

	fd, err = os.OpenFile("s2/testfile.txt", os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		t.Fatal("s1/testfile.txt should not exist")
	}
}

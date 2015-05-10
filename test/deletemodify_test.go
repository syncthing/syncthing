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
	"testing"
)

func TestDeleteModify(t *testing.T) {
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

	err = os.Mkdir("s1/testdir", 0755)
	if err != nil {
		t.Fatal(err)
	}
	fd, err := os.Create("s1/testdir/testfile.txt")
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

	sender, receiver := coSenderReceiver(t)
	defer sender.stop()
	defer receiver.stop()

	if err = coCompletion(sender, receiver); err != nil {
		t.Fatal(err)
	}

	sender.stop()
	receiver.stop()

	log.Println("Verifying...")

	actual, err := directoryContents("s2")
	if err != nil {
		t.Fatal(err)
	}
	err = compareDirectoryContents(actual, expected)
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Introducing a conflict (simultaneous modification and deletion)...")

	err = os.RemoveAll("s1/testdir")
	if err != nil {
		t.Fatal(err)
	}
	fd, err = os.OpenFile("s2/testdir/testfile.txt", os.O_WRONLY|os.O_APPEND, 0644)
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

	fd, err = os.Create("s2/testdir/testfile2.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = fd.WriteString("new file in s2\n")
	if err != nil {
		t.Fatal(err)
	}
	err = fd.Close()
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Syncing...")
	t.Skipf("TODO: deal with stuck puller state")

	err = receiver.start()
	err = sender.start()
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		sender.stop()
		t.Fatal(err)
	}

	if err = coCompletion(sender, receiver); err != nil {
		t.Fatal(err)
	}

	sender.stop()
	receiver.stop()
}

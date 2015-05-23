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
	"testing"
	"time"

	"github.com/syncthing/syncthing/internal/osutil"
)

func TestConflict(t *testing.T) {
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

	sender, receiver := coSenderReceiver(t)
	defer sender.stop()
	defer receiver.stop()

	if err = awaitCompletion("default", sender, receiver); err != nil {
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

	log.Println("Introducing a conflict (simultaneous edit)...")

	fd, err = os.OpenFile("s1/testfile.txt", os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fd.WriteString("text added to s1\n")
	if err != nil {
		t.Fatal(err)
	}
	err = fd.Close()
	if err != nil {
		t.Fatal(err)
	}

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

	log.Println("Syncing...")

	err = receiver.start()
	err = sender.start()
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		sender.stop()
		t.Fatal(err)
	}

	if err = awaitCompletion("default", sender, receiver); err != nil {
		t.Fatal(err)
	}

	sender.stop()
	receiver.stop()

	// The conflict is expected on the s2 side due to how we calculate which
	// file is the winner (based on device ID)

	files, err := osutil.Glob("s2/*sync-conflict*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Errorf("Expected 1 conflicted files instead of %d", len(files))
	}

	log.Println("Introducing a conflict (edit plus delete)...")

	err = os.Remove("s1/testfile.txt")
	if err != nil {
		t.Fatal(err)
	}

	fd, err = os.OpenFile("s2/testfile.txt", os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fd.WriteString("more text added to s2\n")
	if err != nil {
		t.Fatal(err)
	}
	err = fd.Close()
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Syncing...")

	err = receiver.start()
	err = sender.start()
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		sender.stop()
		t.Fatal(err)
	}

	if err = awaitCompletion("default", sender, receiver); err != nil {
		t.Fatal(err)
	}

	sender.stop()
	receiver.stop()

	// The conflict should manifest on the s2 side again, where we should have
	// moved the file to a conflict copy instead of just deleting it.

	files, err = osutil.Glob("s2/*sync-conflict*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("Expected 2 conflicted files instead of %d", len(files))
	}
}

func TestInitialMergeConflicts(t *testing.T) {
	log.Println("Cleaning...")
	err := removeAll("s1", "s2", "h1/index*", "h2/index*")
	if err != nil {
		t.Fatal(err)
	}

	err = os.Mkdir("s1", 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Mkdir("s2", 0755)
	if err != nil {
		t.Fatal(err)
	}

	// File 1 is a conflict

	err = ioutil.WriteFile("s1/file1", []byte("hello\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = ioutil.WriteFile("s2/file1", []byte("goodbye\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// File 2 exists on s1 only

	err = ioutil.WriteFile("s1/file2", []byte("hello\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// File 3 exists on s2 only

	err = ioutil.WriteFile("s2/file3", []byte("goodbye\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Let them sync

	sender, receiver := coSenderReceiver(t)
	defer sender.stop()
	defer receiver.stop()

	log.Println("Syncing...")

	if err = awaitCompletion("default", sender, receiver); err != nil {
		t.Fatal(err)
	}

	sender.stop()
	receiver.stop()

	log.Println("Verifying...")

	// s1 should have three-four files (there's a conflict from s2 which may or may not have synced yet)

	files, err := osutil.Glob("s1/file*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 3 || len(files) > 4 {
		t.Errorf("Expected 3-4 files in s1 instead of %d", len(files))
	}

	// s2 should have four files (there's a conflict)

	files, err = osutil.Glob("s2/file*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 4 {
		t.Errorf("Expected 4 files in s2 instead of %d", len(files))
	}

	// file1 is in conflict, so there's two versions of that one

	files, err = osutil.Glob("s2/file1*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("Expected 2 'file1' files in s2 instead of %d", len(files))
	}
}

func TestResetConflicts(t *testing.T) {
	log.Println("Cleaning...")
	err := removeAll("s1", "s2", "h1/index*", "h2/index*")
	if err != nil {
		t.Fatal(err)
	}

	err = os.Mkdir("s1", 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Mkdir("s2", 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Three files on s1

	err = ioutil.WriteFile("s1/file1", []byte("hello\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = ioutil.WriteFile("s1/file2", []byte("hello\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = ioutil.WriteFile("s2/file3", []byte("hello\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Let them sync

	sender, receiver := coSenderReceiver(t)
	defer sender.stop()
	defer receiver.stop()

	log.Println("Syncing...")

	if err = awaitCompletion("default", sender, receiver); err != nil {
		t.Fatal(err)
	}

	log.Println("Verifying...")

	// s1 should have three files

	files, err := osutil.Glob("s1/file*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Errorf("Expected 3 files in s1 instead of %d", len(files))
	}

	// s2 should have three

	files, err = osutil.Glob("s2/file*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Errorf("Expected 3 files in s2 instead of %d", len(files))
	}

	log.Println("Updating...")

	// change s2/file2 a few times, so that it's version counter increases.
	// This will make the file on the cluster look newer than what we have
	// locally after we rest the index, unless we have a fix for that.

	err = ioutil.WriteFile("s2/file2", []byte("hello1\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = receiver.rescan("default")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	err = ioutil.WriteFile("s2/file2", []byte("hello2\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = receiver.rescan("default")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	err = ioutil.WriteFile("s2/file2", []byte("hello3\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = receiver.rescan("default")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)

	if err = awaitCompletion("default", sender, receiver); err != nil {
		t.Fatal(err)
	}

	// Now nuke the index

	log.Println("Resetting...")

	receiver.stop()
	removeAll("h2/index*")

	// s1/file1 (remote) changes while receiver is down

	err = ioutil.WriteFile("s1/file1", []byte("goodbye\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// s1 must know about it
	err = sender.rescan("default")
	if err != nil {
		t.Fatal(err)
	}

	// s2/file2 (local) changes while receiver is down

	err = ioutil.WriteFile("s2/file2", []byte("goodbye\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	receiver.start()

	log.Println("Syncing...")

	if err = awaitCompletion("default", sender, receiver); err != nil {
		t.Fatal(err)
	}

	// s2 should have five files (three plus two conflicts)

	files, err = osutil.Glob("s2/file*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 5 {
		t.Errorf("Expected 5 files in s2 instead of %d", len(files))
	}

	// file1 is in conflict, so there's two versions of that one

	files, err = osutil.Glob("s2/file1*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("Expected 2 'file1' files in s2 instead of %d", len(files))
	}

	// file2 is in conflict, so there's two versions of that one

	files, err = osutil.Glob("s2/file2*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("Expected 2 'file2' files in s2 instead of %d", len(files))
	}
}

func coSenderReceiver(t *testing.T) (syncthingProcess, syncthingProcess) {
	log.Println("Starting sender...")
	sender := syncthingProcess{ // id1
		instance: "1",
		argv:     []string{"-home", "h1"},
		port:     8081,
		apiKey:   apiKey,
	}
	err := sender.start()
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Starting receiver...")
	receiver := syncthingProcess{ // id2
		instance: "2",
		argv:     []string{"-home", "h2"},
		port:     8082,
		apiKey:   apiKey,
	}
	err = receiver.start()
	if err != nil {
		sender.stop()
		t.Fatal(err)
	}

	return sender, receiver
}

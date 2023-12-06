// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build integration
// +build integration

package integration

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/rc"
)

func TestConflictsDefault(t *testing.T) {
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

	sender := startInstance(t, 1)
	defer checkedStop(t, sender)
	receiver := startInstance(t, 2)
	defer checkedStop(t, receiver)

	sender.ResumeAll()
	receiver.ResumeAll()

	// Rescan with a delay on the next one, so we are not surprised by a
	// sudden rescan while we're trying to introduce conflicts.

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

	log.Println("Introducing a conflict (simultaneous edit)...")

	if err := sender.PauseDevice(receiver.ID()); err != nil {
		t.Fatal(err)
	}

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

	if err := sender.ResumeDevice(receiver.ID()); err != nil {
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

	// Expect one conflict file, created on either side.

	files, err := filepath.Glob("s?/*sync-conflict*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("Expected 1 conflicted file on each side, instead of totally %d", len(files))
	} else if filepath.Base(files[0]) != filepath.Base(files[1]) {
		t.Errorf(`Expected same conflicted file on both sides, got "%v" and "%v"`, files[0], files[1])
	}

	log.Println("Introducing a conflict (edit plus delete)...")

	if err := sender.PauseDevice(receiver.ID()); err != nil {
		t.Fatal(err)
	}

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

	if err := sender.ResumeDevice(receiver.ID()); err != nil {
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

	// The conflict is resolved to the advantage of the edit over the delete.
	// As such, we get the edited content synced back to s1 where it was
	// removed.

	files, err = filepath.Glob("s2/*sync-conflict*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Errorf("Expected 1 conflicted files instead of %d", len(files))
	}
	bs, err := os.ReadFile("s1/testfile.txt")
	if err != nil {
		t.Error("reading file:", err)
	}
	if !bytes.Contains(bs, []byte("more text added to s2")) {
		t.Error("s1/testfile.txt should contain data added in s2")
	}
}

func TestConflictsInitialMerge(t *testing.T) {
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

	err = os.WriteFile("s1/file1", []byte("hello\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile("s2/file1", []byte("goodbye\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// File 2 exists on s1 only

	err = os.WriteFile("s1/file2", []byte("hello\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// File 3 exists on s2 only

	err = os.WriteFile("s2/file3", []byte("goodbye\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Let them sync

	sender := startInstance(t, 1)
	defer checkedStop(t, sender)
	receiver := startInstance(t, 2)
	defer checkedStop(t, receiver)

	sender.ResumeAll()
	receiver.ResumeAll()

	log.Println("Syncing...")

	rc.AwaitSync("default", sender, receiver)

	// Do it once more so the conflict copies propagate to both sides.

	sender.Rescan("default")
	receiver.Rescan("default")

	rc.AwaitSync("default", sender, receiver)

	checkedStop(t, sender)
	checkedStop(t, receiver)

	log.Println("Verifying...")

	// s1 should have four files (there's a conflict)

	files, err := filepath.Glob("s1/file*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 4 {
		t.Errorf("Expected 4 files in s1 instead of %d", len(files))
	}

	// s2 should have four files (there's a conflict)

	files, err = filepath.Glob("s2/file*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 4 {
		t.Errorf("Expected 4 files in s2 instead of %d", len(files))
	}

	// file1 is in conflict, so there's two versions of that one

	files, err = filepath.Glob("s2/file1*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("Expected 2 'file1' files in s2 instead of %d", len(files))
	}
}

func TestConflictsIndexReset(t *testing.T) {
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

	err = os.WriteFile("s1/file1", []byte("hello\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile("s1/file2", []byte("hello\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile("s2/file3", []byte("hello\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Let them sync

	sender := startInstance(t, 1)
	defer checkedStop(t, sender)
	receiver := startInstance(t, 2)
	defer checkedStop(t, receiver)

	sender.ResumeAll()
	receiver.ResumeAll()

	log.Println("Syncing...")

	rc.AwaitSync("default", sender, receiver)

	log.Println("Verifying...")

	// s1 should have three files

	files, err := filepath.Glob("s1/file*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Errorf("Expected 3 files in s1 instead of %d", len(files))
	}

	// s2 should have three files

	files, err = filepath.Glob("s2/file*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Errorf("Expected 3 files in s2 instead of %d", len(files))
	}

	log.Println("Updating...")

	// change s2/file2 a few times, so that its version counter increases.
	// This will make the file on the cluster look newer than what we have
	// locally after we rest the index, unless we have a fix for that.

	for i := 0; i < 5; i++ {
		err = os.WriteFile("s2/file2", []byte("hello1\n"), 0644)
		if err != nil {
			t.Fatal(err)
		}
		err = receiver.Rescan("default")
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Second)
	}

	rc.AwaitSync("default", sender, receiver)

	// Now nuke the index

	log.Println("Resetting...")

	checkedStop(t, receiver)
	removeAll("h2/index*")

	// s1/file1 (remote) changes while receiver is down

	err = os.WriteFile("s1/file1", []byte("goodbye\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// s1 must know about it
	err = sender.Rescan("default")
	if err != nil {
		t.Fatal(err)
	}

	// s2/file2 (local) changes while receiver is down

	err = os.WriteFile("s2/file2", []byte("goodbye\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	receiver = startInstance(t, 2)
	defer checkedStop(t, receiver)
	receiver.ResumeAll()

	log.Println("Syncing...")

	rc.AwaitSync("default", sender, receiver)

	// s2 should have five files (three plus two conflicts)

	files, err = filepath.Glob("s2/file*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 5 {
		t.Errorf("Expected 5 files in s2 instead of %d", len(files))
	}

	// file1 is in conflict, so there's two versions of that one

	files, err = filepath.Glob("s2/file1*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("Expected 2 'file1' files in s2 instead of %d", len(files))
	}

	// file2 is in conflict, so there's two versions of that one

	files, err = filepath.Glob("s2/file2*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("Expected 2 'file2' files in s2 instead of %d", len(files))
	}
}

func TestConflictsSameContent(t *testing.T) {
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

	// Two files on s1

	err = os.WriteFile("s1/file1", []byte("hello\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile("s1/file2", []byte("hello\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Two files on s2, content differs in file1 only, timestamps differ on both.

	err = os.WriteFile("s2/file1", []byte("goodbye\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile("s2/file2", []byte("hello\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	ts := time.Now().Add(-time.Hour)
	os.Chtimes("s2/file1", ts, ts)
	os.Chtimes("s2/file2", ts, ts)

	// Let them sync

	sender := startInstance(t, 1)
	defer checkedStop(t, sender)
	receiver := startInstance(t, 2)
	defer checkedStop(t, receiver)

	sender.ResumeAll()
	receiver.ResumeAll()

	log.Println("Syncing...")

	rc.AwaitSync("default", sender, receiver)

	// Let conflict copies propagate

	sender.Rescan("default")
	receiver.Rescan("default")
	rc.AwaitSync("default", sender, receiver)

	log.Println("Verifying...")

	// s1 should have three files

	files, err := filepath.Glob("s1/file*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Errorf("Expected 3 files in s1 instead of %d", len(files))
	}

	// s2 should have three files

	files, err = filepath.Glob("s2/file*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Errorf("Expected 3 files in s2 instead of %d", len(files))
	}
}

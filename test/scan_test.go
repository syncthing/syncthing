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

	"github.com/syncthing/syncthing/lib/rc"
)

func TestScanSubdir(t *testing.T) {
	log.Println("Cleaning...")
	err := removeAll("s1", "s2", "h1/index*", "h2/index*")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generating files...")
	err = generateFiles("s1", 10, 10, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Scan a single file in a known directory "file1.txt"
	// 2. Scan a single file in an unknown directory "filetest/file1.txt"
	// 3. Scan a single file in a deep unknown directory "filetest/1/2/3/4/5/6/7/file1.txt"
	// 4. Scan a directory in a deep unknown directory "dirtest/1/2/3/4/5/6/7"
	// 5. Scan a deleted file in a known directory "filetest/file1.txt"
	// 6. Scan a deleted file in a deep unknown directory "rmdirtest/1/2/3/4/5/6/7"
	// 7. 'Accidentally' forget to scan 1 of the 2 files in a known directory

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

	// 1
	log.Println("Creating new file...")
	if fd, err := os.Create("s1/file1.txt"); err != nil {
		t.Fatal(err)
	} else {
		fd.Close()
	}
	if err := sender.RescanSub("default", "file1.txt", 86400); err != nil {
		t.Fatal(err)
	}
	log.Println("Syncing...")
	rc.AwaitSync("default", sender, receiver)
	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}

	// 2
	log.Println("Creating a file in an unknown directory")
	os.MkdirAll("s1/filetest", 0o755)
	if fd, err := os.Create("s1/filetest/file1.txt"); err != nil {
		t.Fatal(err)
	} else {
		fd.Close()
	}
	if err := sender.RescanSub("default", "filetest/file1.txt", 86400); err != nil {
		t.Fatal(err)
	}
	log.Println("Syncing...")
	rc.AwaitSync("default", sender, receiver)
	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}

	// 3
	log.Println("Creating a file in an unknown deep directory")
	os.MkdirAll("s1/filetest/1/2/3/4/5/6/7", 0o755)
	if fd, err := os.Create("s1/filetest/1/2/3/4/5/6/7/file1.txt"); err != nil {
		t.Fatal(err)
	} else {
		fd.Close()
	}
	if err := sender.RescanSub("default", "filetest/1/2/3/4/5/6/7/file1.txt", 86400); err != nil {
		t.Fatal(err)
	}
	log.Println("Syncing...")
	rc.AwaitSync("default", sender, receiver)
	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}

	// 4
	log.Println("Creating a directory in an unknown directory")
	err = os.MkdirAll("s1/dirtest/1/2/3/4/5/6/7", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.RescanSub("default", "dirtest/1/2/3/4/5/6/7", 86400); err != nil {
		t.Fatal(err)
	}
	log.Println("Syncing...")
	rc.AwaitSync("default", sender, receiver)
	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}

	// 5
	log.Println("Scan a deleted file in a known directory")
	if err := os.Remove("s1/filetest/file1.txt"); err != nil {
		t.Fatal(err)
	}
	if err := sender.RescanSub("default", "filetest/file1.txt", 86400); err != nil {
		t.Fatal(err)
	}
	log.Println("Syncing...")
	rc.AwaitSync("default", sender, receiver)
	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}

	// 6
	log.Println("Scan a deleted file in an unknown directory")
	if err := sender.RescanSub("default", "rmdirtest/1/2/3/4/5/6/7", 86400); err != nil {
		t.Fatal(err)
	}
	log.Println("Syncing...")
	rc.AwaitSync("default", sender, receiver)
	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}

	// 7
	log.Println("'Accidentally' forget to scan 1 of the 2 files")
	if fd, err := os.Create("s1/filetest/1/2/3/4/5/6/7/file2.txt"); err != nil {
		t.Fatal(err)
	} else {
		fd.Close()
	}
	if fd, err := os.Create("s1/filetest/1/2/3/4/5/6/7/file3.txt"); err != nil {
		t.Fatal(err)
	} else {
		fd.Close()
	}
	if err := sender.RescanSub("default", "filetest/1/2/3/4/5/6/7/file2.txt", 86400); err != nil {
		t.Fatal(err)
	}
	log.Println("Syncing...")
	rc.AwaitSync("default", sender, receiver)
	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err == nil {
		t.Fatal("filetest/1/2/3/4/5/6/7/file3.txt should not be synced")
	}
	os.Remove("s1/filetest/1/2/3/4/5/6/7/file3.txt")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}
}

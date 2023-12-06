// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build integration
// +build integration

package integration

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/rc"
)

const (
	longTimeLimit  = 1 * time.Minute
	shortTimeLimit = 25 * time.Second
	s12Folder      = `¯\_(ツ)_/¯ Räksmörgås 动作 Адрес` // This was renamed to ensure arbitrary folder IDs are fine.
)

func TestSyncCluster(t *testing.T) {
	// This tests syncing files back and forth between three cluster members.
	// Their configs are in h1, h2 and h3. The folder "default" is shared
	// between all and stored in s1, s2 and s3 respectively.
	//
	// Another folder is shared between 1 and 2 only, in s12-1 and s12-2. A
	// third folders is shared between 2 and 3, in s23-2 and s23-3.

	// When -short is passed, keep it more reasonable.
	timeLimit := longTimeLimit
	if testing.Short() {
		timeLimit = shortTimeLimit
	}

	const (
		numFiles    = 100
		fileSizeExp = 20
	)
	rand.Seed(42)

	log.Printf("Testing with numFiles=%d, fileSizeExp=%d, timeLimit=%v", numFiles, fileSizeExp, timeLimit)

	log.Println("Cleaning...")
	err := removeAll("s1", "s12-1",
		"s2", "s12-2", "s23-2",
		"s3", "s23-3",
		"h1/index*", "h2/index*", "h3/index*")
	if err != nil {
		t.Fatal(err)
	}

	// Create initial folder contents. All three devices have stuff in
	// "default", which should be merged. The other two folders are initially
	// empty on one side.

	log.Println("Generating files...")

	err = generateFiles("s1", numFiles, fileSizeExp, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	err = generateFiles("s12-1", numFiles, fileSizeExp, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}

	// We'll use this file for appending data without modifying the time stamp.
	fd, err := os.Create("s1/test-appendfile")
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

	err = generateFiles("s2", numFiles, fileSizeExp, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	err = generateFiles("s23-2", numFiles, fileSizeExp, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}

	err = generateFiles("s3", numFiles, fileSizeExp, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}

	// Prepare the expected state of folders after the sync
	c1, err := directoryContents("s1")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := directoryContents("s2")
	if err != nil {
		t.Fatal(err)
	}
	c3, err := directoryContents("s3")
	if err != nil {
		t.Fatal(err)
	}
	e1 := mergeDirectoryContents(c1, c2, c3)
	e2, err := directoryContents("s12-1")
	if err != nil {
		t.Fatal(err)
	}
	e3, err := directoryContents("s23-2")
	if err != nil {
		t.Fatal(err)
	}
	expected := [][]fileInfo{e1, e2, e3}

	// Start the syncers

	log.Println("Starting Syncthing...")

	p0 := startInstance(t, 1)
	defer checkedStop(t, p0)
	p1 := startInstance(t, 2)
	defer checkedStop(t, p1)
	p2 := startInstance(t, 3)
	defer checkedStop(t, p2)

	p0.ResumeAll()
	p1.ResumeAll()
	p2.ResumeAll()

	p := []*rc.Process{p0, p1, p2}

	start := time.Now()
	iteration := 0
	for time.Since(start) < timeLimit {
		iteration++
		log.Println("Iteration", iteration)

		log.Println("Forcing rescan...")

		// Force rescan of folders
		for i, device := range p {
			if err := device.RescanDelay("default", 86400); err != nil {
				t.Fatal(err)
			}
			if i == 0 || i == 1 {
				if err := device.RescanDelay(s12Folder, 86400); err != nil {
					t.Fatal(err)
				}
			}
			if i == 1 || i == 2 {
				if err := device.RescanDelay("s23", 86400); err != nil {
					t.Fatal(err)
				}
			}
		}

		// Sync stuff and verify it looks right
		err = scSyncAndCompare(p, expected)
		if err != nil {
			t.Error(err)
			break
		}

		// Sleep for a little over a second to ensure that this round of
		// alterations ends up in a different second than the previous one,
		// even if the sync was quick. This is to give Syncthing a visible
		// mtime change even on filesystem with whole second resolution.
		time.Sleep(1100 * time.Millisecond)

		log.Println("Altering...")

		// Alter the source files for another round
		err = alterFiles("s1")
		if err != nil {
			t.Error(err)
			break
		}
		err = alterFiles("s12-1")
		if err != nil {
			t.Error(err)
			break
		}
		err = alterFiles("s23-2")
		if err != nil {
			t.Error(err)
			break
		}

		// Alter the "test-appendfile" without changing its modification time. Sneaky!
		fi, err := os.Stat("s1/test-appendfile")
		if err != nil {
			t.Fatal(err)
		}
		fd, err := os.OpenFile("s1/test-appendfile", os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			t.Fatal(err)
		}
		_, err = fd.Seek(0, os.SEEK_END)
		if err != nil {
			t.Fatal(err)
		}
		_, err = fd.WriteString("more data\n")
		if err != nil {
			t.Fatal(err)
		}
		err = fd.Close()
		if err != nil {
			t.Fatal(err)
		}
		err = os.Chtimes("s1/test-appendfile", fi.ModTime(), fi.ModTime())
		if err != nil {
			t.Fatal(err)
		}

		// Prepare the expected state of folders after the sync
		e1, err = directoryContents("s1")
		if err != nil {
			t.Fatal(err)
		}
		e2, err = directoryContents("s12-1")
		if err != nil {
			t.Fatal(err)
		}
		e3, err = directoryContents("s23-2")
		if err != nil {
			t.Fatal(err)
		}
		expected = [][]fileInfo{e1, e2, e3}
	}
}

func scSyncAndCompare(p []*rc.Process, expected [][]fileInfo) error {
	log.Println("Syncing...")

	for {
		time.Sleep(250 * time.Millisecond)
		if !rc.InSync("default", p...) {
			continue
		}
		if !rc.InSync(s12Folder, p[0], p[1]) {
			continue
		}
		if !rc.InSync("s23", p[1], p[2]) {
			continue
		}
		break
	}

	log.Println("Checking...")

	for _, dir := range []string{"s1", "s2", "s3"} {
		actual, err := directoryContents(dir)
		if err != nil {
			return err
		}
		if err := compareDirectoryContents(actual, expected[0]); err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
	}

	if len(expected) > 1 {
		for _, dir := range []string{"s12-1", "s12-2"} {
			actual, err := directoryContents(dir)
			if err != nil {
				return err
			}
			if err := compareDirectoryContents(actual, expected[1]); err != nil {
				return fmt.Errorf("%s: %w", dir, err)
			}
		}
	}

	if len(expected) > 2 {
		for _, dir := range []string{"s23-2", "s23-3"} {
			actual, err := directoryContents(dir)
			if err != nil {
				return err
			}
			if err := compareDirectoryContents(actual, expected[2]); err != nil {
				return fmt.Errorf("%s: %w", dir, err)
			}
		}
	}

	if err := checkRemoteInSync("default", p[0], p[1]); err != nil {
		return err
	}
	if err := checkRemoteInSync("default", p[0], p[2]); err != nil {
		return err
	}
	if err := checkRemoteInSync(s12Folder, p[0], p[1]); err != nil {
		return err
	}
	if err := checkRemoteInSync("s23", p[1], p[2]); err != nil {
		return err
	}

	return nil
}

func TestSyncSparseFile(t *testing.T) {
	// This test verifies that when syncing a file that consists mostly of
	// zeroes, those blocks are not transferred. It doesn't verify whether
	// the resulting file is actually *sparse* or not.

	log.Println("Cleaning...")
	err := removeAll("s1", "s12-1",
		"s2", "s12-2", "s23-2",
		"s3", "s23-3",
		"h1/index*", "h2/index*", "h3/index*")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generating files...")

	if err := os.Mkdir("s1", 0755); err != nil {
		t.Fatal(err)
	}

	fd, err := os.Create("s1/testfile")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fd.Write([]byte("Start")); err != nil {
		t.Fatal(err)
	}
	kib := make([]byte, 1024)
	for i := 0; i < 8192; i++ {
		if _, err := fd.Write(kib); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fd.Write([]byte("End")); err != nil {
		t.Fatal(err)
	}
	fd.Close()

	// Start the syncers

	log.Println("Syncing...")

	p0 := startInstance(t, 1)
	defer checkedStop(t, p0)
	p1 := startInstance(t, 2)
	defer checkedStop(t, p1)

	p0.ResumeAll()
	p1.ResumeAll()

	rc.AwaitSync("default", p0, p1)

	log.Println("Comparing...")

	if err := compareDirectories("s1", "s2"); err != nil {
		t.Fatal(err)
	}

	conns, err := p0.Connections()
	if err != nil {
		t.Fatal(err)
	}

	tot := conns["total"]
	if tot.OutBytesTotal > 256<<10 {
		t.Fatal("Sending side has sent", tot.OutBytesTotal, "bytes, which is too much")
	}
}

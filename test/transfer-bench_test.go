// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build integration && benchmark
// +build integration,benchmark

package integration

import (
	"log"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/rc"
)

func TestBenchmarkTransferManyFiles(t *testing.T) {
	setupAndBenchmarkTransfer(t, 10000, 15)
}

func TestBenchmarkTransferLargeFile1G(t *testing.T) {
	setupAndBenchmarkTransfer(t, 1, 30)
}

func TestBenchmarkTransferLargeFile2G(t *testing.T) {
	setupAndBenchmarkTransfer(t, 1, 31)
}

func TestBenchmarkTransferLargeFile4G(t *testing.T) {
	setupAndBenchmarkTransfer(t, 1, 32)
}

func TestBenchmarkTransferLargeFile8G(t *testing.T) {
	setupAndBenchmarkTransfer(t, 1, 33)
}

func TestBenchmarkTransferLargeFile16G(t *testing.T) {
	setupAndBenchmarkTransfer(t, 1, 34)
}

func TestBenchmarkTransferLargeFile32G(t *testing.T) {
	setupAndBenchmarkTransfer(t, 1, 35)
}

func setupAndBenchmarkTransfer(t *testing.T, files, sizeExp int) {
	cleanBenchmarkTransfer(t)

	log.Println("Generating files...")
	var err error
	if files == 1 {
		// Special case. Generate one file with the specified size exactly.
		var fd *os.File
		fd, err = os.Open("../LICENSE")
		if err != nil {
			t.Fatal(err)
		}
		err = os.MkdirAll("s1", 0o755)
		if err != nil {
			t.Fatal(err)
		}
		err = generateOneFile(fd, "s1/onefile", 1<<uint(sizeExp), time.Now())
	} else {
		err = generateFiles("s1", files, sizeExp, "../LICENSE")
	}
	if err != nil {
		t.Fatal(err)
	}

	benchmarkTransfer(t)
}

// TestBenchmarkTransferSameFiles doesn't actually transfer anything, but tests
// how fast two devices get in sync if they have the same data locally.
func TestBenchmarkTransferSameFiles(t *testing.T) {
	cleanBenchmarkTransfer(t)

	t0 := time.Now()
	rand.Seed(0)
	log.Println("Generating files in s1...")
	if err := generateFilesWithTime("s1", 10000, 10, "../LICENSE", t0); err != nil {
		t.Fatal(err)
	}

	rand.Seed(0)
	log.Println("Generating same files in s2...")
	if err := generateFilesWithTime("s2", 10000, 10, "../LICENSE", t0); err != nil {
		t.Fatal(err)
	}

	benchmarkTransfer(t)
}

func benchmarkTransfer(t *testing.T) {
	expected, err := directoryContents("s1")
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	var nfiles int
	for _, f := range expected {
		total += f.size
		if f.mode.IsRegular() {
			nfiles++
		}
	}
	log.Printf("Total %.01f MiB in %d files", float64(total)/1024/1024, nfiles)

	sender := startInstance(t, 1)
	defer checkedStop(t, sender)
	receiver := startInstance(t, 2)
	defer checkedStop(t, receiver)

	sender.ResumeAll()
	receiver.ResumeAll()

	t0 := time.Now()
	var t1 time.Time
	lastEvent := 0

loop:
	for {
		evs, err := receiver.Events(lastEvent)
		if err != nil {
			if isTimeout(err) {
				continue
			}
			t.Fatal(err)
		}

		for _, ev := range evs {
			lastEvent = ev.ID

			switch ev.Type {
			case "ItemFinished":
				break loop
			}
		}

		time.Sleep(250 * time.Millisecond)
	}

	processes := []*rc.Process{sender, receiver}
	for {
		if rc.InSync("default", processes...) {
			t1 = time.Now()
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	sendProc, err := sender.Stop()
	if err != nil {
		t.Fatal(err)
	}
	recvProc, err := receiver.Stop()
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

	log.Printf("Result: Wall time: %v / MiB", t1.Sub(t0)/time.Duration(total/1024/1024))
	log.Printf("Result: %.3g KiB/s synced", float64(total)/1024/t1.Sub(t0).Seconds())

	printUsage("Receiver", recvProc, total)
	printUsage("Sender", sendProc, total)

	cleanBenchmarkTransfer(t)
}

func cleanBenchmarkTransfer(t *testing.T) {
	log.Println("Cleaning...")
	err := removeAll("s1", "s2", "h1/index*", "h2/index*")
	if err != nil {
		t.Fatal(err)
	}
}

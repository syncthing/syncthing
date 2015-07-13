// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build integration,benchmark

package integration

import (
	"log"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
)

func TestBenchmarkTransferManyFiles(t *testing.T) {
	benchmarkTransfer(t, 50000, 15)
}

func TestBenchmarkTransferLargeFile1G(t *testing.T) {
	benchmarkTransfer(t, 1, 30)
}
func TestBenchmarkTransferLargeFile2G(t *testing.T) {
	benchmarkTransfer(t, 1, 31)
}
func TestBenchmarkTransferLargeFile4G(t *testing.T) {
	benchmarkTransfer(t, 1, 32)
}
func TestBenchmarkTransferLargeFile8G(t *testing.T) {
	benchmarkTransfer(t, 1, 33)
}
func TestBenchmarkTransferLargeFile16G(t *testing.T) {
	benchmarkTransfer(t, 1, 34)
}
func TestBenchmarkTransferLargeFile32G(t *testing.T) {
	benchmarkTransfer(t, 1, 35)
}

func benchmarkTransfer(t *testing.T, files, sizeExp int) {
	log.Println("Cleaning...")
	err := removeAll("s1", "s2", "h1/index*", "h2/index*")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generating files...")
	if files == 1 {
		// Special case. Generate one file with the specified size exactly.
		fd, err := os.Open("../LICENSE")
		if err != nil {
			t.Fatal(err)
		}
		err = os.MkdirAll("s1", 0755)
		if err != nil {
			t.Fatal(err)
		}
		err = generateOneFile(fd, "s1/onefile", 1<<uint(sizeExp))
	} else {
		err = generateFiles("s1", files, sizeExp, "../LICENSE")
	}
	if err != nil {
		t.Fatal(err)
	}
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

	var t0, t1 time.Time
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
			if ev.Type == "StateChanged" {
				data := ev.Data.(map[string]interface{})
				if data["folder"].(string) != "default" {
					continue
				}
				log.Println(ev)
				if data["to"].(string) == "syncing" {
					t0 = ev.Time
					continue
				}
				if !t0.IsZero() && data["to"].(string) == "idle" {
					t1 = ev.Time
					break loop
				}
			}
			lastEvent = ev.ID
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

	log.Println("Result: Wall time:", t1.Sub(t0))
	log.Printf("Result: %.1f MiB/s synced", float64(total)/1024/1024/t1.Sub(t0).Seconds())

	if rusage, ok := recvProc.SysUsage().(*syscall.Rusage); ok {
		log.Println("Receiver: Utime:", time.Duration(rusage.Utime.Nano()))
		log.Println("Receiver: Stime:", time.Duration(rusage.Stime.Nano()))
		if runtime.GOOS == "darwin" {
			// Darwin reports in bytes, Linux seems to report in KiB even
			// though the manpage says otherwise.
			rusage.Maxrss /= 1024
		}
		log.Println("Receiver: MaxRSS:", rusage.Maxrss, "KiB")
	}
	if rusage, ok := sendProc.SysUsage().(*syscall.Rusage); ok {
		log.Println("Sender: Utime:", time.Duration(rusage.Utime.Nano()))
		log.Println("Sender: Stime:", time.Duration(rusage.Stime.Nano()))
		if runtime.GOOS == "darwin" {
			// Darwin reports in bytes, Linux seems to report in KiB even
			// though the manpage says otherwise.
			rusage.Maxrss /= 1024
		}
		log.Println("Sender: MaxRSS:", rusage.Maxrss, "KiB")
	}
}

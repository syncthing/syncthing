// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build integration,benchmark

package integration

import (
	"log"
	"syscall"
	"testing"
	"time"
)

func TestBenchmarkTransferManyFiles(t *testing.T) {
	benchmarkTransfer(t, 50000, 15)
}

func TestBenchmarkTransferLargeFiles(t *testing.T) {
	benchmarkTransfer(t, 200, 24)
}

func benchmarkTransfer(t *testing.T, files, sizeExp int) {
	log.Println("Cleaning...")
	err := removeAll("s1", "s2", "h1/index*", "h2/index*")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generating files...")
	err = generateFiles("s1", files, sizeExp, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := directoryContents("s1")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Starting sender...")
	sender := syncthingProcess{ // id1
		instance: "1",
		argv:     []string{"-home", "h1"},
		port:     8081,
		apiKey:   apiKey,
	}
	err = sender.start()
	if err != nil {
		t.Fatal(err)
	}

	// Wait for one scan to succeed, or up to 20 seconds... This is to let
	// startup, UPnP etc complete and make sure the sender has the full index
	// before they connect.
	for i := 0; i < 20; i++ {
		resp, err := sender.post("/rest/scan?folder=default", nil)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			time.Sleep(time.Second)
			continue
		}
		break
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

	var t0, t1 time.Time
loop:
	for {
		evs, err := receiver.events()
		if err != nil {
			if isTimeout(err) {
				continue
			}
			sender.stop()
			receiver.stop()
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
		}

		time.Sleep(250 * time.Millisecond)
	}

	sender.stop()
	proc, err := receiver.stop()
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

	if rusage, ok := proc.SysUsage().(*syscall.Rusage); ok {
		log.Println("Result: Utime:", time.Duration(rusage.Utime.Nano()))
		log.Println("Result: Stime:", time.Duration(rusage.Stime.Nano()))
		log.Println("Result: MaxRSS:", rusage.Maxrss/1024, "KiB")
	}
}

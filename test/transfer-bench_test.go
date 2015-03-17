// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build integration,benchmark

package integration

import (
	"log"
	"testing"
	"time"
)

func TestBenchmarkTransfer(t *testing.T) {
	log.Println("Cleaning...")
	err := removeAll("s1", "s2", "h1/index", "h2/index")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generating files...")
	err = generateFiles("s1", 10000, 22, "../LICENSE")
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

	// Make sure the sender has the full index before they connect
	sender.post("/rest/scan?folder=default", nil)

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

	log.Println("Sync took", t1.Sub(t0))
}

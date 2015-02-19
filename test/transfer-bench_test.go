// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

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

// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build integration

package integration_test

import (
	"log"
	"strings"
	"testing"
	"time"
)

func TestBenchmarkTransfer(t *testing.T) {
	nfiles := 10000
	if testing.Short() {
		nfiles = 1000
	}

	log.Println("Cleaning...")
	err := removeAll("s1", "s2", "h1/index", "h2/index")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generating files...")
	err = generateFiles("s1", nfiles, 22, "../bin/syncthing")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Starting up...")
	sender := syncthingProcess{ // id1
		log:    "1.out",
		argv:   []string{"-home", "h1"},
		port:   8081,
		apiKey: apiKey,
	}
	ver, err := sender.start()
	if err != nil {
		t.Fatal(err)
	}
	log.Println(ver)

	receiver := syncthingProcess{ // id2
		log:    "2.out",
		argv:   []string{"-home", "h2"},
		port:   8082,
		apiKey: apiKey,
	}
	ver, err = receiver.start()
	if err != nil {
		sender.stop()
		t.Fatal(err)
	}
	log.Println(ver)

	var t0 time.Time
loop:
	for {
		evs, err := receiver.events()
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				log.Println("...")
				continue
			}
			sender.stop()
			receiver.stop()
			t.Fatal(err)
		}

		for _, ev := range evs {
			if ev.Type == "StateChanged" {
				data := ev.Data.(map[string]interface{})
				if data["repo"].(string) != "default" {
					continue
				}
				log.Println(ev)
				if data["to"].(string) == "syncing" {
					t0 = ev.Time
					continue
				}
				if t0 != (time.Time{}) && data["to"].(string) == "idle" {
					log.Println("Sync took", ev.Time.Sub(t0))
					break loop
				}
			}
		}
	}

	sender.stop()
	receiver.stop()
}

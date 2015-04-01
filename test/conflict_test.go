// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build integration

package integration

import (
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"
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
	defer sender.stop()

	// Wait for one scan to succeed, or up to 20 seconds... This is to let
	// startup, UPnP etc complete and make sure the sender has the full index
	// before they connect.
	for i := 0; i < 20; i++ {
		err := sender.rescan("default")
		if err != nil {
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
	defer receiver.stop()

	if err = coCompletion(sender, receiver); err != nil {
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

	if err = coCompletion(sender, receiver); err != nil {
		t.Fatal(err)
	}

	sender.stop()
	receiver.stop()

	// The conflict is expected on the s2 side due to how we calculate which
	// file is the winner (based on device ID)

	files, err := filepath.Glob("s2/*sync-conflict*")
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

	if err = coCompletion(sender, receiver); err != nil {
		t.Fatal(err)
	}

	sender.stop()
	receiver.stop()

	// The conflict should manifest on the s2 side again, where we should have
	// moved the file to a conflict copy instead of just deleting it.

	files, err = filepath.Glob("s2/*sync-conflict*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("Expected 2 conflicted files instead of %d", len(files))
	}
}

func coCompletion(p ...syncthingProcess) error {
mainLoop:
	for {
		time.Sleep(2500 * time.Millisecond)

		tot := 0
		for i := range p {
			comp, err := p[i].peerCompletion()
			if err != nil {
				if isTimeout(err) {
					continue mainLoop
				}
				return err
			}

			for _, pct := range comp {
				tot += pct
			}
		}

		if tot == 100*(len(p)) {
			return nil
		}

		log.Printf("%d / %d...", tot, 100*(len(p)))
	}
}

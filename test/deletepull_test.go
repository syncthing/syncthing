// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build integration

package integration

import (
	"log"
	"os"
	"testing"
	"time"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/internal/config"
)

func TestDeletePull(t *testing.T) {
	// This tests what happens if device 3 is partially synchronized while
	// device 1 removes the entire folder
	// Their configs are in h1, h2 and h3. The folder "default" is shared
	// between all and stored in s1, s2 and s3 respectively.

	const (
		numFiles    = 100
		fileSizeExp = 25
	)
	log.Printf("Testing with numFiles=%d, fileSizeExp=%d", numFiles, fileSizeExp)

	log.Println("Cleaning...")
	err := removeAll("s1", "s12-1",
		"s2", "s12-2", "s23-2",
		"s3", "s23-3",
		"h1/index*", "h2/index*", "h3/index*")
	if err != nil {
		t.Fatal(err)
	}

	// Disable versioning
	id, _ := protocol.DeviceIDFromString(id2)
	cfg, _ := config.Load("h2/config.xml", id)
	fld := cfg.Folders()["default"]
	fld.Versioning = config.VersioningConfiguration{}
	cfg.SetFolder(fld)
	cfg.Save()
	id, _ = protocol.DeviceIDFromString(id3)
	cfg, _ = config.Load("h3/config.xml", id)
	fld = cfg.Folders()["default"]
	fld.Versioning = config.VersioningConfiguration{}
	cfg.SetFolder(fld)
	cfg.Save()

	// Create initial folder contents. Device 1 contains all data in
	// "default", which should be synced.
	// The other two devices are initially empty

	log.Println("Generating files...")

	err = generateFiles("s1/testdir/dira/dirb/dirc", numFiles, fileSizeExp, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}

	// Prepare the expected state of folders after the sync
	expected, err := directoryContents("s1")
	if err != nil {
		t.Fatal(err)
	}

	// Start device 1 and 2
	p := make([]syncthingProcess, 2)
	p[0] = syncthingProcess{ // id1
		instance: "1",
		argv:     []string{"-home", "h1"},
		port:     8081,
		apiKey:   apiKey,
	}
	err = p[0].start()
	if err != nil {
		t.Fatal(err)
	}
	p[1] = syncthingProcess{ // id2
		instance: "2",
		argv:     []string{"-home", "h2"},
		port:     8082,
		apiKey:   apiKey,
	}
	err = p[1].start()
	if err != nil {
		p[0].stop()
		t.Fatal(err)
	}
	defer func() {
		for i := range p {
			p[i].stop()
		}
	}()

	log.Println("Waiting for startup...")
	// Wait for one scan to succeed, or up to 20 seconds...
	// This is to let startup, UPnP etc complete.
	for _, device := range p {
		for i := 0; i < 20; i++ {
			err := device.rescan("default")
			if err != nil {
				time.Sleep(time.Second)
				continue
			}
			break
		}
	}

	log.Println("Forcing rescan...")

	// Force rescan of folders
	for _, device := range p {
		if err := device.rescan("default"); err != nil {
			t.Fatal(err)
		}
	}

	// Sync stuff and verify it looks right
	log.Println("Syncing...")

	coCompletion(p[0], p[1])
	actual, err := directoryContents("s2")
	if err != nil {
		t.Fatal(err)
	}
	err = compareDirectoryContents(actual, expected)
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Starting device 3...")
	device3 := syncthingProcess{ // id3
		instance: "3",
		argv:     []string{"-home", "h3"},
		port:     8083,
		apiKey:   apiKey,
	}
	err = device3.start()
	if err != nil {
		t.Fatal(err)
	}
	p = append(p, device3)

	// Wait a little bit until files start to appear
	for i := 0; i < 100; i++ {
		if _, err := os.Stat("s3/testdir/dira/dirb/dirc"); os.IsNotExist(err) {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		log.Println("Device 3 is partially synced...")
		break
	}

	log.Println("Removing directory...")

	// Remove the entire directory
	err = os.RemoveAll("s1/testdir/dira/dirb/dirc")
	if err != nil {
		t.Error(err)
	}
	expected, err = directoryContents("s1")
	if err != nil {
		t.Fatal(err)
	}
	if err := p[0].rescan("default"); err != nil {
		t.Fatal(err)
	}

	// Sync stuff and verify it looks right
	log.Println("Syncing...")

	coCompletion(p[0], p[1], p[2])
	// BUG
	time.Sleep(5 * time.Second)
	coCompletion(p[0], p[1], p[2])
	// BUG
	time.Sleep(5 * time.Second)

	actual, err = directoryContents("s2")
	if err != nil {
		t.Fatal(err)
	}
	err = compareDirectoryContents(actual, expected)
	if err != nil {
		t.Fatal(err)
	}
	actual, err = directoryContents("s3")
	if err != nil {
		t.Fatal(err)
	}
	err = compareDirectoryContents(actual, expected)
	if err != nil {
		t.Fatal(err)
	}
}

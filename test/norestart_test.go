// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build integration

package integration

import (
	"log"
	"os"
	"testing"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rc"
)

func TestAddDeviceWithoutRestart(t *testing.T) {
	log.Println("Cleaning...")
	err := removeAll("s1", "h1/index*", "s4", "h4/index*")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generating files...")
	err = generateFiles("s1", 100, 18, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}

	p1 := startInstance(t, 1)
	defer checkedStop(t, p1)

	p4 := startInstance(t, 4)
	defer checkedStop(t, p4)

	if ok, err := p1.ConfigInSync(); err != nil || !ok {
		t.Fatal("p1 should be in sync;", ok, err)
	}
	if ok, err := p4.ConfigInSync(); err != nil || !ok {
		t.Fatal("p4 should be in sync;", ok, err)
	}

	// Add the p1 device to p4. Back up and restore p4's config first.

	log.Println("Adding p1 to p4...")

	os.Remove("h4/config.xml.orig")
	os.Rename("h4/config.xml", "h4/config.xml.orig")
	defer osutil.Rename("h4/config.xml.orig", "h4/config.xml")

	cfg, err := p4.GetConfig()
	if err != nil {
		t.Fatal(err)
	}

	devCfg := config.DeviceConfiguration{
		DeviceID:    p1.ID(),
		Name:        "s1",
		Addresses:   []string{"127.0.0.1:22001"},
		Compression: protocol.CompressMetadata,
	}
	cfg.Devices = append(cfg.Devices, devCfg)

	cfg.Folders[0].Devices = append(cfg.Folders[0].Devices, config.FolderDeviceConfiguration{DeviceID: p1.ID()})

	if err = p4.PostConfig(cfg); err != nil {
		t.Fatal(err)
	}

	// The change should not require a restart, so the config should be "in sync"

	if ok, err := p4.ConfigInSync(); err != nil || !ok {
		t.Fatal("p4 should be in sync;", ok, err)
	}

	// Wait for the devices to connect and sync.

	log.Println("Waiting for p1 and p4 to connect and sync...")

	rc.AwaitSync("default", p1, p4)
}

func TestFolderWithoutRestart(t *testing.T) {
	log.Println("Cleaning...")
	err := removeAll("testfolder-p1", "testfolder-p4", "h1/index*", "h4/index*")
	if err != nil {
		t.Fatal(err)
	}
	defer removeAll("testfolder-p1", "testfolder-p4")

	if err := generateFiles("testfolder-p1", 50, 18, "../LICENSE"); err != nil {
		t.Fatal(err)
	}

	p1 := startInstance(t, 1)
	defer checkedStop(t, p1)

	p4 := startInstance(t, 4)
	defer checkedStop(t, p4)

	if ok, err := p1.ConfigInSync(); err != nil || !ok {
		t.Fatal("p1 should be in sync;", ok, err)
	}

	if ok, err := p4.ConfigInSync(); err != nil || !ok {
		t.Fatal("p4 should be in sync;", ok, err)
	}

	// Add a new folder to p1, shared with p4. Back up and restore the config
	// first.

	log.Println("Adding testfolder to p1...")

	os.Remove("h1/config.xml.orig")
	os.Rename("h1/config.xml", "h1/config.xml.orig")
	defer osutil.Rename("h1/config.xml.orig", "h1/config.xml")

	cfg, err := p1.GetConfig()
	if err != nil {
		t.Fatal(err)
	}

	newFolder := config.FolderConfiguration{
		ID:              "testfolder",
		RawPath:         "testfolder-p1",
		RescanIntervalS: 86400,
		Copiers:         1,
		Hashers:         1,
		Pullers:         1,
		Devices:         []config.FolderDeviceConfiguration{{DeviceID: p4.ID()}},
	}
	newDevice := config.DeviceConfiguration{
		DeviceID:    p4.ID(),
		Name:        "p4",
		Addresses:   []string{"dynamic"},
		Compression: protocol.CompressMetadata,
	}

	cfg.Folders = append(cfg.Folders, newFolder)
	cfg.Devices = append(cfg.Devices, newDevice)

	if err = p1.PostConfig(cfg); err != nil {
		t.Fatal(err)
	}

	// Add a new folder to p4, shared with p1. Back up and restore the config
	// first.

	log.Println("Adding testfolder to p4...")

	os.Remove("h4/config.xml.orig")
	os.Rename("h4/config.xml", "h4/config.xml.orig")
	defer osutil.Rename("h4/config.xml.orig", "h4/config.xml")

	cfg, err = p4.GetConfig()
	if err != nil {
		t.Fatal(err)
	}

	newFolder.RawPath = "testfolder-p4"
	newFolder.Devices = []config.FolderDeviceConfiguration{{DeviceID: p1.ID()}}
	newDevice.DeviceID = p1.ID()
	newDevice.Name = "p1"
	newDevice.Addresses = []string{"127.0.0.1:22001"}

	cfg.Folders = append(cfg.Folders, newFolder)
	cfg.Devices = append(cfg.Devices, newDevice)

	if err = p4.PostConfig(cfg); err != nil {
		t.Fatal(err)
	}

	// The change should not require a restart, so the config should be "in sync"

	if ok, err := p1.ConfigInSync(); err != nil || !ok {
		t.Fatal("p1 should be in sync;", ok, err)
	}
	if ok, err := p4.ConfigInSync(); err != nil || !ok {
		t.Fatal("p4 should be in sync;", ok, err)
	}

	// The folder should start and scan - wait for the event that signals this
	// has happened.

	log.Println("Waiting for testfolder to scan...")

	since := 0
outer:
	for {
		events, err := p4.Events(since)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if event.Type == "StateChanged" {
				data := event.Data.(map[string]interface{})
				folder := data["folder"].(string)
				from := data["from"].(string)
				to := data["to"].(string)
				if folder == "testfolder" && from == "scanning" && to == "idle" {
					break outer
				}
			}
			since = event.ID
		}
	}

	// It should sync to the other side successfully

	log.Println("Waiting for p1 and p4 to connect and sync...")

	rc.AwaitSync("testfolder", p1, p4)
}

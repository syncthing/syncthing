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

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/internal/config"
	"github.com/syncthing/syncthing/internal/rc"
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
	defer os.Rename("h4/config.xml.orig", "h4/config.xml")

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

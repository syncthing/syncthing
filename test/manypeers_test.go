// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build integration
// +build integration

package integration

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"testing"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rc"
)

func TestManyPeers(t *testing.T) {
	log.Println("Cleaning...")
	err := removeAll("s1", "s2", "h1/index*", "h2/index*")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generating files...")
	err = generateFiles("s1", 200, 20, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}

	receiver := startInstance(t, 2)
	defer checkedStop(t, receiver)
	receiver.ResumeAll()

	bs, err := receiver.Get("/rest/system/config")
	if err != nil {
		t.Fatal(err)
	}

	var cfg config.Configuration
	if err := json.Unmarshal(bs, &cfg); err != nil {
		t.Fatal(err)
	}

	for len(cfg.Devices) < 100 {
		bs := make([]byte, 16)
		ReadRand(bs)
		id := protocol.NewDeviceID(bs)
		cfg.Devices = append(cfg.Devices, config.DeviceConfiguration{DeviceID: id})
		cfg.Folders[0].Devices = append(cfg.Folders[0].Devices, config.FolderDeviceConfiguration{DeviceID: id})
	}

	os.Rename("h2/config.xml", "h2/config.xml.orig")
	defer os.Rename("h2/config.xml.orig", "h2/config.xml")

	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(cfg)
	_, err = receiver.Post("/rest/system/config", &buf)
	if err != nil {
		t.Fatal(err)
	}

	sender := startInstance(t, 1)
	defer checkedStop(t, sender)
	sender.ResumeAll()

	rc.AwaitSync("default", sender, receiver)

	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}
}

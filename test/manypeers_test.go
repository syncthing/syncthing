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

// +build integration

package integration

import (
	"bytes"
	"encoding/json"
	"log"
	"testing"
	"time"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/internal/config"
	"github.com/syncthing/syncthing/internal/osutil"
)

func TestManyPeers(t *testing.T) {
	log.Println("Cleaning...")
	err := removeAll("s1", "s2", "h1/index", "h2/index")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("Generating files...")
	err = generateFiles("s1", 200, 20, "../LICENSE")
	if err != nil {
		t.Fatal(err)
	}

	receiver := syncthingProcess{ // id2
		instance: "2",
		argv:     []string{"-home", "h2"},
		port:     8082,
		apiKey:   apiKey,
	}
	err = receiver.start()
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.stop()

	resp, err := receiver.get("/rest/config")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("Code %d != 200", resp.StatusCode)
	}

	var cfg config.Configuration
	json.NewDecoder(resp.Body).Decode(&cfg)
	resp.Body.Close()

	for len(cfg.Devices) < 100 {
		bs := make([]byte, 16)
		ReadRand(bs)
		id := protocol.NewDeviceID(bs)
		cfg.Devices = append(cfg.Devices, config.DeviceConfiguration{DeviceID: id})
		cfg.Folders[0].Devices = append(cfg.Folders[0].Devices, config.FolderDeviceConfiguration{DeviceID: id})
	}

	osutil.Rename("h2/config.xml", "h2/config.xml.orig")
	defer osutil.Rename("h2/config.xml.orig", "h2/config.xml")

	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(cfg)
	resp, err = receiver.post("/rest/config", &buf)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("Code %d != 200", resp.StatusCode)
	}
	resp.Body.Close()

	log.Println("Starting up...")
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

	for {
		comp, err := sender.peerCompletion()
		if err != nil {
			if isTimeout(err) {
				time.Sleep(250 * time.Millisecond)
				continue
			}
			t.Fatal(err)
		}
		if comp[id2] == 100 {
			return
		}
		time.Sleep(2 * time.Second)
	}

	log.Println("Comparing directories...")
	err = compareDirectories("s1", "s2")
	if err != nil {
		t.Fatal(err)
	}
}

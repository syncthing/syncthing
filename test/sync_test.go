// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package integration

import (
	"fmt"
	"testing"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/rc"
)

func TestSyncOneSideToOther(t *testing.T) {
	t.Parallel()

	// Create a source folder with some data in it.
	srcDir := generateTree(t, 100)
	// Create a destination folder to hold the synced data.
	dstDir := t.TempDir()

	// Spin up two instances to sync the data.
	testSyncTwoDevicesFolders(t, srcDir, dstDir)

	// Check that the destination folder now contains the same files as the source folder.
	compareTrees(t, srcDir, dstDir)
}

func TestSyncMergeTwoDevices(t *testing.T) {
	t.Parallel()

	// Create a source folder with some data in it.
	srcDir := generateTree(t, 50)
	// Create a destination folder that also has some data in it.
	dstDir := generateTree(t, 50)

	// Spin up two instances to sync the data.
	testSyncTwoDevicesFolders(t, srcDir, dstDir)

	// Check that the destination folder now contains the same files as the source folder.
	if total := compareTrees(t, srcDir, dstDir); total != 100 {
		t.Fatalf("expected 100 files, got %d", total)
	}
}

func testSyncTwoDevicesFolders(t *testing.T, srcDir, dstDir string) {
	t.Helper()

	// The folder needs an ID.
	folderID := rand.String(8)

	// Start the source device.
	src := startInstance(t)
	srcAPI := rc.NewAPI(src.apiAddress, src.apiKey)

	// Start the destination device.
	dst := startInstance(t)
	dstAPI := rc.NewAPI(dst.apiAddress, dst.apiKey)

	// Add the peer device to each device. Hard code the sync addresses to
	// speed things up.
	if err := srcAPI.Post("/rest/config/devices", &config.DeviceConfiguration{
		DeviceID:  dst.deviceID,
		Addresses: []string{fmt.Sprintf("tcp://127.0.0.1:%d", dst.tcpPort)},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := dstAPI.Post("/rest/config/devices", &config.DeviceConfiguration{
		DeviceID:  src.deviceID,
		Addresses: []string{fmt.Sprintf("tcp://127.0.0.1:%d", src.tcpPort)},
	}, nil); err != nil {
		t.Fatal(err)
	}

	// Add the folder to both devices.
	if err := srcAPI.Post("/rest/config/folders", &config.FolderConfiguration{
		ID:             folderID,
		Path:           srcDir,
		FilesystemType: fs.FilesystemTypeBasic,
		Type:           config.FolderTypeSendReceive,
		Devices: []config.FolderDeviceConfiguration{
			{DeviceID: src.deviceID},
			{DeviceID: dst.deviceID},
		},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := dstAPI.Post("/rest/config/folders", &config.FolderConfiguration{
		ID:             folderID,
		Path:           dstDir,
		FilesystemType: fs.FilesystemTypeBasic,
		Type:           config.FolderTypeSendReceive,
		Devices: []config.FolderDeviceConfiguration{
			{DeviceID: src.deviceID},
			{DeviceID: dst.deviceID},
		},
	}, nil); err != nil {
		t.Fatal(err)
	}

	// Listen to events; we want to know when the folder is fully synced. We
	// consider the other side in sync when we've received an index update
	// from them and subsequently a completion event with percentage equal
	// to 100. At that point they should be done. Wait for both sides to do
	// their thing.

	waitForSync := func(api *rc.API, done chan struct{}) {
		defer close(done)
		lastEventID := 0
		remoteCompletion := 0.0
		remoteIndexUpdated := false
	loop:
		for {
			events, err := api.Events(lastEventID)
			if err != nil {
				t.Log(err)
				break loop
			}

			for _, ev := range events {
				lastEventID = ev.ID
				switch ev.Type {
				case "RemoteIndexUpdated":
					data := ev.Data.(map[string]any)
					folder := data["folder"].(string)
					if folder != folderID {
						continue
					}
					remoteIndexUpdated = true
					remoteCompletion = 0.0
				case "FolderCompletion":
					data := ev.Data.(map[string]any)
					folder := data["folder"].(string)
					if folder != folderID {
						continue
					}
					remoteCompletion = data["completion"].(float64)
				}
				if remoteIndexUpdated && remoteCompletion == 100.0 {
					break loop
				}
			}
		}
	}

	srcDone := make(chan struct{})
	go waitForSync(srcAPI, srcDone)
	dstDone := make(chan struct{})
	go waitForSync(dstAPI, dstDone)

	<-srcDone
	<-dstDone
}

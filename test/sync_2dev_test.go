// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package integration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/rc"
)

// TestSyncOneSideToOther verifies that files on one side get synced to the
// other. The test creates actual files on disk in a temp directory, so that
// the data can be compared after syncing.
func TestSyncOneSideToOther(t *testing.T) {
	t.Parallel()

	// Create a source folder with some data in it.
	srcDir := generateTree(t, 100)
	// Create an empty destination folder to hold the synced data.
	dstDir := t.TempDir()

	ctx := context.Background()
	if dl, ok := t.Deadline(); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, dl)
		defer cancel()
	}

	// Spin up two instances to sync the data.
	testSyncTwoDevicesFolders(ctx, t, srcDir, dstDir)

	// Check that the destination folder now contains the same files as the source folder.
	compareTrees(t, srcDir, dstDir)
}

// TestSyncMergeTwoDevices verifies that two sets of files, one from each
// device, get merged into a coherent total. The test creates actual files
// on disk in a temp directory, so that the data can be compared after
// syncing.
func TestSyncMergeTwoDevices(t *testing.T) {
	t.Parallel()

	// Create a source folder with some data in it.
	srcDir := generateTree(t, 50)
	// Create a destination folder that also has some data in it.
	dstDir := generateTree(t, 50)

	ctx := context.Background()
	if dl, ok := t.Deadline(); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, dl)
		defer cancel()
	}

	// Spin up two instances to sync the data.
	testSyncTwoDevicesFolders(ctx, t, srcDir, dstDir)

	// Check that both folders are the same, and the file count should be
	// the sum of the two.
	if total := compareTrees(t, srcDir, dstDir); total != 100 {
		t.Fatalf("expected 100 files, got %d", total)
	}
}

func testSyncTwoDevicesFolders(ctx context.Context, t *testing.T, srcDir, dstDir string) {
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

	var srcDur, dstDur map[string]time.Duration

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		var err error
		srcDur, err = waitForSync(ctx, folderID, srcAPI)
		if err != nil {
			t.Error("src:", err)
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		dstDur, err = waitForSync(ctx, folderID, dstAPI)
		if err != nil {
			t.Error("dst:", err)
		}
	}()
	wg.Wait()

	t.Log("src durations:", srcDur)
	t.Log("dst durations:", dstDur)
}

// waitForSync waits for the folder with the given ID to be fully synced.
// There is a race condition; if the folder is already in sync when we
// start, the events leading up to that have been forgotten, and nothing
// happens thereafter, we may wait forever.
func waitForSync(ctx context.Context, folderID string, api *rc.API) (map[string]time.Duration, error) {
	lastEventID := 0
	indexUpdated := false
	completed := false
	var completedWhen time.Time
	stateTimes := make(map[string]time.Duration)
	for {
		// Get events, with a five second timeout.
		ectx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		evs, err := api.Events(ectx, lastEventID)
		if errors.Is(err, context.DeadlineExceeded) {
			// Check if the main context is done, or if it was just our
			// shorter timeout.
			select {
			case <-ctx.Done():
				return stateTimes, ctx.Err()
			default:
			}
		} else if err != nil {
			return stateTimes, err
		}

		// If we're completed and it's been a while without hearing
		// otherwise, we're done.
		if indexUpdated && completed && time.Since(completedWhen) > 4*time.Second {
			return stateTimes, nil
		}

		for _, ev := range evs {
			lastEventID = ev.ID
			switch ev.Type {
			case events.StateChanged.String():
				data := ev.Data.(map[string]any)
				folder := data["folder"].(string)
				if folder != folderID {
					continue
				}
				from := data["from"].(string)
				if duration, dok := data["duration"].(float64); dok {
					stateTimes[from] += time.Duration(duration * float64(time.Second))
				}

			case events.RemoteIndexUpdated.String():
				data := ev.Data.(map[string]any)
				folder := data["folder"].(string)
				if folder != folderID {
					continue
				}
				completed = false
				indexUpdated = true

			case events.FolderCompletion.String():
				data := ev.Data.(map[string]any)
				folder := data["folder"].(string)
				if folder != folderID {
					continue
				}
				completed = data["completion"].(float64) == 100.0
				if completed {
					completedWhen = ev.Time
				}
			}
		}
	}
}

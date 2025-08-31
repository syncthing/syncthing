// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
)

var timeout = 100 * time.Millisecond

func caller(skip int) string {
	_, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		return "unknown"
	}
	return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}

func expectEvent(w events.Subscription, t *testing.T, size int) {
	event, err := w.Poll(timeout)
	if err != nil {
		t.Fatal("Unexpected error:", err, "at", caller(1))
	}
	if event.Type != events.DownloadProgress {
		t.Fatal("Unexpected event:", event, "at", caller(1))
	}
	data := event.Data.(map[string]map[string]*PullerProgress)
	if len(data) != size {
		t.Fatal("Unexpected event data size:", data, "at", caller(1))
	}
}

func expectTimeout(w events.Subscription, t *testing.T) {
	_, err := w.Poll(timeout)
	if err != events.ErrTimeout {
		t.Fatal("Unexpected non-Timeout error:", err, "at", caller(1))
	}
}

func TestProgressEmitter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	evLogger := events.NewLogger()
	go evLogger.Serve(ctx)
	defer cancel()

	w := evLogger.Subscribe(events.DownloadProgress)

	c, cfgCancel := newConfigWrapper(config.Configuration{Version: config.CurrentVersion})
	defer os.Remove(c.ConfigPath())
	defer cfgCancel()
	waiter, err := c.Modify(func(cfg *config.Configuration) {
		cfg.Options.ProgressUpdateIntervalS = 60 // irrelevant, but must be positive
	})
	if err != nil {
		t.Fatal(err)
	}
	waiter.Wait()

	p := NewProgressEmitter(c, evLogger)
	go p.Serve(ctx)
	p.interval = 0

	expectTimeout(w, t)

	s := sharedPullerState{
		updated: time.Now(),
	}
	p.Register(&s)

	expectEvent(w, t, 1)
	expectTimeout(w, t)

	s.copyDone(protocol.BlockInfo{})

	expectEvent(w, t, 1)
	expectTimeout(w, t)

	s.copiedFromOrigin(1)

	expectEvent(w, t, 1)
	expectTimeout(w, t)

	s.pullStarted()

	expectEvent(w, t, 1)
	expectTimeout(w, t)

	s.pullDone(protocol.BlockInfo{})

	expectEvent(w, t, 1)
	expectTimeout(w, t)

	p.Deregister(&s)

	expectEvent(w, t, 0)
	expectTimeout(w, t)
}

func TestSendDownloadProgressMessages(t *testing.T) {
	c, cfgCancel := newConfigWrapper(config.Configuration{Version: config.CurrentVersion})
	defer os.Remove(c.ConfigPath())
	defer cfgCancel()
	waiter, err := c.Modify(func(cfg *config.Configuration) {
		cfg.Options.ProgressUpdateIntervalS = 60 // irrelevant, but must be positive
		cfg.Options.TempIndexMinBlocks = 10
	})
	if err != nil {
		t.Fatal(err)
	}
	waiter.Wait()

	fc := newFakeConnection(protocol.DeviceID{}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	evLogger := events.NewLogger()
	go evLogger.Serve(ctx)
	defer cancel()

	p := NewProgressEmitter(c, evLogger)
	p.temporaryIndexSubscribe(fc, []string{"folder", "folder2"})
	p.registry["folder"] = make(map[string]*sharedPullerState)
	p.registry["folder2"] = make(map[string]*sharedPullerState)
	p.registry["folderXXX"] = make(map[string]*sharedPullerState)

	expect := func(updateIdx int, state *sharedPullerState, updateType protocol.FileDownloadProgressUpdateType, version protocol.Vector, blocks []int, remove bool) {
		messageIdx := -1
		for i, msg := range fc.downloadProgressMessages {
			if msg.folder == state.folder {
				messageIdx = i
				break
			}
		}
		if messageIdx < 0 {
			t.Errorf("Message for folder %s does not exist at %s", state.folder, caller(1))
		}

		msg := fc.downloadProgressMessages[messageIdx]

		// Don't know the index (it's random due to iterating maps)
		if updateIdx == -1 {
			for i, upd := range msg.updates {
				if upd.Name == state.file.Name {
					updateIdx = i
					break
				}
			}
		}

		if updateIdx == -1 {
			t.Errorf("Could not find update for %s at %s", state.file.Name, caller(1))
		}

		if updateIdx > len(msg.updates)-1 {
			t.Errorf("Update at index %d does not exist at %s", updateIdx, caller(1))
		}

		update := msg.updates[updateIdx]

		if update.UpdateType != updateType {
			t.Errorf("Wrong update type at %s", caller(1))
		}

		if !update.Version.Equal(version) {
			t.Errorf("Wrong version at %s", caller(1))
		}

		if len(update.BlockIndexes) != len(blocks) {
			t.Errorf("Wrong indexes. Have %d expect %d at %s", len(update.BlockIndexes), len(blocks), caller(1))
		}
		for i := range update.BlockIndexes {
			if update.BlockIndexes[i] != blocks[i] {
				t.Errorf("Index %d incorrect at %s", i, caller(1))
			}
		}

		if remove {
			fc.downloadProgressMessages = append(fc.downloadProgressMessages[:messageIdx], fc.downloadProgressMessages[messageIdx+1:]...)
		}
	}
	expectEmpty := func() {
		if len(fc.downloadProgressMessages) > 0 {
			t.Errorf("Still have something at %s: %#v", caller(1), fc.downloadProgressMessages)
		}
	}

	now := time.Now()
	tick := func() time.Time {
		now = now.Add(time.Second)
		return now
	}

	if len(fc.downloadProgressMessages) != 0 {
		t.Error("Expected no requests")
	}

	v1 := (protocol.Vector{}).Update(0)
	v2 := (protocol.Vector{}).Update(1)

	// Requires more than 10 blocks to work.
	blocks := make([]protocol.BlockInfo, 11)

	state1 := &sharedPullerState{
		folder: "folder",
		file: protocol.FileInfo{
			Name:    "state1",
			Version: v1,
			Blocks:  blocks,
		},
		availableUpdated: time.Now(),
	}
	p.registry["folder"]["1"] = state1

	// Has no blocks, hence no message is sent
	sendMsgs(p)
	expectEmpty()

	// Returns update for puller with new extra blocks
	state1.available = []int{1}
	sendMsgs(p)

	expect(0, state1, protocol.FileDownloadProgressUpdateTypeAppend, v1, []int{1}, true)
	expectEmpty()

	// Does nothing if nothing changes
	sendMsgs(p)
	expectEmpty()

	// Does nothing if timestamp updated, but no new blocks (should never happen)
	state1.availableUpdated = tick()

	sendMsgs(p)
	expectEmpty()

	// Does not return an update if date blocks change but date does not (should never happen)
	state1.available = []int{1, 2}

	sendMsgs(p)
	expectEmpty()

	// If the date and blocks changes, returns only the diff
	state1.availableUpdated = tick()

	sendMsgs(p)

	expect(0, state1, protocol.FileDownloadProgressUpdateTypeAppend, v1, []int{2}, true)
	expectEmpty()

	// Returns forget and update if puller version has changed
	state1.file.Version = v2

	sendMsgs(p)

	expect(0, state1, protocol.FileDownloadProgressUpdateTypeForget, v1, nil, false)
	expect(1, state1, protocol.FileDownloadProgressUpdateTypeAppend, v2, []int{1, 2}, true)
	expectEmpty()

	// Returns forget and append if sharedPullerState creation timer changes.

	state1.available = []int{1}
	state1.availableUpdated = tick()
	state1.created = tick()

	sendMsgs(p)

	expect(0, state1, protocol.FileDownloadProgressUpdateTypeForget, v2, nil, false)
	expect(1, state1, protocol.FileDownloadProgressUpdateTypeAppend, v2, []int{1}, true)
	expectEmpty()

	// Sends an empty update if new file exists, but does not have any blocks yet. (To indicate that the old blocks are no longer available)
	state1.file.Version = v1
	state1.available = nil
	state1.availableUpdated = tick()

	sendMsgs(p)

	expect(0, state1, protocol.FileDownloadProgressUpdateTypeForget, v2, nil, false)
	expect(1, state1, protocol.FileDownloadProgressUpdateTypeAppend, v1, nil, true)
	expectEmpty()

	// Updates for multiple files and folders can be combined
	state1.available = []int{1, 2, 3}
	state1.availableUpdated = tick()

	state2 := &sharedPullerState{
		folder: "folder2",
		file: protocol.FileInfo{
			Name:    "state2",
			Version: v1,
			Blocks:  blocks,
		},
		available:        []int{1, 2, 3},
		availableUpdated: time.Now(),
	}
	state3 := &sharedPullerState{
		folder: "folder",
		file: protocol.FileInfo{
			Name:    "state3",
			Version: v1,
			Blocks:  blocks,
		},
		available:        []int{1, 2, 3},
		availableUpdated: time.Now(),
	}
	state4 := &sharedPullerState{
		folder: "folder2",
		file: protocol.FileInfo{
			Name:    "state4",
			Version: v1,
			Blocks:  blocks,
		},
		available:        []int{1, 2, 3},
		availableUpdated: time.Now(),
	}
	p.registry["folder2"]["2"] = state2
	p.registry["folder"]["3"] = state3
	p.registry["folder2"]["4"] = state4

	sendMsgs(p)

	expect(-1, state1, protocol.FileDownloadProgressUpdateTypeAppend, v1, []int{1, 2, 3}, false)
	expect(-1, state3, protocol.FileDownloadProgressUpdateTypeAppend, v1, []int{1, 2, 3}, true)
	expect(-1, state2, protocol.FileDownloadProgressUpdateTypeAppend, v1, []int{1, 2, 3}, false)
	expect(-1, state4, protocol.FileDownloadProgressUpdateTypeAppend, v1, []int{1, 2, 3}, true)
	expectEmpty()

	// Returns forget if puller no longer exists, as well as updates if it has been updated.
	state1.available = []int{1, 2, 3, 4, 5}
	state1.availableUpdated = tick()
	state2.available = []int{1, 2, 3, 4, 5}
	state2.availableUpdated = tick()

	delete(p.registry["folder"], "3")
	delete(p.registry["folder2"], "4")

	sendMsgs(p)

	expect(-1, state1, protocol.FileDownloadProgressUpdateTypeAppend, v1, []int{4, 5}, false)
	expect(-1, state3, protocol.FileDownloadProgressUpdateTypeForget, v1, nil, true)
	expect(-1, state2, protocol.FileDownloadProgressUpdateTypeAppend, v1, []int{4, 5}, false)
	expect(-1, state4, protocol.FileDownloadProgressUpdateTypeForget, v1, nil, true)
	expectEmpty()

	// Deletions are sent only once (actual bug I found writing the tests)
	sendMsgs(p)
	sendMsgs(p)
	expectEmpty()

	// Not sent for "inactive" (symlinks, dirs, or wrong folder) pullers
	// Directory
	state5 := &sharedPullerState{
		folder: "folder",
		file: protocol.FileInfo{
			Name:    "state5",
			Version: v1,
			Type:    protocol.FileInfoTypeDirectory,
			Blocks:  blocks,
		},
		available:        []int{1, 2, 3},
		availableUpdated: time.Now(),
	}
	// Symlink
	state6 := &sharedPullerState{
		folder: "folder",
		file: protocol.FileInfo{
			Name:    "state6",
			Version: v1,
			Type:    protocol.FileInfoTypeSymlink,
		},
		available:        []int{1, 2, 3},
		availableUpdated: time.Now(),
	}
	// Some other directory
	state7 := &sharedPullerState{
		folder: "folderXXX",
		file: protocol.FileInfo{
			Name:    "state7",
			Version: v1,
			Blocks:  blocks,
		},
		available:        []int{1, 2, 3},
		availableUpdated: time.Now(),
	}
	// Less than 10 blocks
	state8 := &sharedPullerState{
		folder: "folder",
		file: protocol.FileInfo{
			Name:    "state8",
			Version: v1,
			Blocks:  blocks[:3],
		},
		available:        []int{1, 2, 3},
		availableUpdated: time.Now(),
	}
	p.registry["folder"]["5"] = state5
	p.registry["folder"]["6"] = state6
	p.registry["folderXXX"]["7"] = state7
	p.registry["folder"]["8"] = state8

	sendMsgs(p)

	expectEmpty()

	// Device is no longer subscribed to a particular folder
	delete(p.registry["folder"], "1")  // Clean up first
	delete(p.registry["folder2"], "2") // Clean up first

	sendMsgs(p)
	expect(-1, state1, protocol.FileDownloadProgressUpdateTypeForget, v1, nil, true)
	expect(-1, state2, protocol.FileDownloadProgressUpdateTypeForget, v1, nil, true)

	expectEmpty()

	p.registry["folder"]["1"] = state1
	p.registry["folder2"]["2"] = state2
	p.registry["folder"]["3"] = state3
	p.registry["folder2"]["4"] = state4

	sendMsgs(p)

	expect(-1, state1, protocol.FileDownloadProgressUpdateTypeAppend, v1, []int{1, 2, 3, 4, 5}, false)
	expect(-1, state3, protocol.FileDownloadProgressUpdateTypeAppend, v1, []int{1, 2, 3}, true)
	expect(-1, state2, protocol.FileDownloadProgressUpdateTypeAppend, v1, []int{1, 2, 3, 4, 5}, false)
	expect(-1, state4, protocol.FileDownloadProgressUpdateTypeAppend, v1, []int{1, 2, 3}, true)
	expectEmpty()

	p.temporaryIndexUnsubscribe(fc)
	p.temporaryIndexSubscribe(fc, []string{"folder"})

	sendMsgs(p)

	// See progressemitter.go for explanation why this is commented out.
	// Search for state.cleanup
	// expect(-1, state2, protocol.FileDownloadProgressUpdateTypeForget, v1, nil, false)
	// expect(-1, state4, protocol.FileDownloadProgressUpdateTypeForget, v1, nil, true)

	expectEmpty()

	// Cleanup when device no longer exists
	p.temporaryIndexUnsubscribe(fc)

	sendMsgs(p)
	_, ok := p.sentDownloadStates[fc.DeviceID()]
	if ok {
		t.Error("Should not be there")
	}
}

func sendMsgs(p *ProgressEmitter) {
	p.mut.Lock()
	updates := p.computeProgressUpdates()
	p.mut.Unlock()
	ctx := context.Background()
	for _, update := range updates {
		update.send(ctx)
	}
}

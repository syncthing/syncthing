// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
	"github.com/syncthing/syncthing/lib/sync"
)

func init() {
	// We do this to make sure that the temp file required for the tests does
	// not get removed during the tests.
	future := time.Now().Add(time.Hour)
	err := os.Chtimes(filepath.Join("testdata", defTempNamer.TempName("file")), future, future)
	if err != nil {
		panic(err)
	}
}

var blocks = []protocol.BlockInfo{
	{Hash: []uint8{0xfa, 0x43, 0x23, 0x9b, 0xce, 0xe7, 0xb9, 0x7c, 0xa6, 0x2f, 0x0, 0x7c, 0xc6, 0x84, 0x87, 0x56, 0xa, 0x39, 0xe1, 0x9f, 0x74, 0xf3, 0xdd, 0xe7, 0x48, 0x6d, 0xb3, 0xf9, 0x8d, 0xf8, 0xe4, 0x71}}, // Zero'ed out block
	{Offset: 0, Size: 0x20000, Hash: []uint8{0x7e, 0xad, 0xbc, 0x36, 0xae, 0xbb, 0xcf, 0x74, 0x43, 0xe2, 0x7a, 0x5a, 0x4b, 0xb8, 0x5b, 0xce, 0xe6, 0x9e, 0x1e, 0x10, 0xf9, 0x8a, 0xbc, 0x77, 0x95, 0x2, 0x29, 0x60, 0x9e, 0x96, 0xae, 0x6c}},
	{Offset: 131072, Size: 0x20000, Hash: []uint8{0x3c, 0xc4, 0x20, 0xf4, 0xb, 0x2e, 0xcb, 0xb9, 0x5d, 0xce, 0x34, 0xa8, 0xc3, 0x92, 0xea, 0xf3, 0xda, 0x88, 0x33, 0xee, 0x7a, 0xb6, 0xe, 0xf1, 0x82, 0x5e, 0xb0, 0xa9, 0x26, 0xa9, 0xc0, 0xef}},
	{Offset: 262144, Size: 0x20000, Hash: []uint8{0x76, 0xa8, 0xc, 0x69, 0xd7, 0x5c, 0x52, 0xfd, 0xdf, 0x55, 0xef, 0x44, 0xc1, 0xd6, 0x25, 0x48, 0x4d, 0x98, 0x48, 0x4d, 0xaa, 0x50, 0xf6, 0x6b, 0x32, 0x47, 0x55, 0x81, 0x6b, 0xed, 0xee, 0xfb}},
	{Offset: 393216, Size: 0x20000, Hash: []uint8{0x44, 0x1e, 0xa4, 0xf2, 0x8d, 0x1f, 0xc3, 0x1b, 0x9d, 0xa5, 0x18, 0x5e, 0x59, 0x1b, 0xd8, 0x5c, 0xba, 0x7d, 0xb9, 0x8d, 0x70, 0x11, 0x5c, 0xea, 0xa1, 0x57, 0x4d, 0xcb, 0x3c, 0x5b, 0xf8, 0x6c}},
	{Offset: 524288, Size: 0x20000, Hash: []uint8{0x8, 0x40, 0xd0, 0x5e, 0x80, 0x0, 0x0, 0x7c, 0x8b, 0xb3, 0x8b, 0xf7, 0x7b, 0x23, 0x26, 0x28, 0xab, 0xda, 0xcf, 0x86, 0x8f, 0xc2, 0x8a, 0x39, 0xc6, 0xe6, 0x69, 0x59, 0x97, 0xb6, 0x1a, 0x43}},
	{Offset: 655360, Size: 0x20000, Hash: []uint8{0x38, 0x8e, 0x44, 0xcb, 0x30, 0xd8, 0x90, 0xf, 0xce, 0x7, 0x4b, 0x58, 0x86, 0xde, 0xce, 0x59, 0xa2, 0x46, 0xd2, 0xf9, 0xba, 0xaf, 0x35, 0x87, 0x38, 0xdf, 0xd2, 0xd, 0xf9, 0x45, 0xed, 0x91}},
	{Offset: 786432, Size: 0x20000, Hash: []uint8{0x32, 0x28, 0xcd, 0xf, 0x37, 0x21, 0xe5, 0xd4, 0x1e, 0x58, 0x87, 0x73, 0x8e, 0x36, 0xdf, 0xb2, 0x70, 0x78, 0x56, 0xc3, 0x42, 0xff, 0xf7, 0x8f, 0x37, 0x95, 0x0, 0x26, 0xa, 0xac, 0x54, 0x72}},
	{Offset: 917504, Size: 0x20000, Hash: []uint8{0x96, 0x6b, 0x15, 0x6b, 0xc4, 0xf, 0x19, 0x18, 0xca, 0xbb, 0x5f, 0xd6, 0xbb, 0xa2, 0xc6, 0x2a, 0xac, 0xbb, 0x8a, 0xb9, 0xce, 0xec, 0x4c, 0xdb, 0x78, 0xec, 0x57, 0x5d, 0x33, 0xf9, 0x8e, 0xaf}},
}

var folders = []string{"default"}

func setUpFile(filename string, blockNumbers []int) protocol.FileInfo {
	// Create existing file
	existingBlocks := make([]protocol.BlockInfo, len(blockNumbers))
	for i := range blockNumbers {
		existingBlocks[i] = blocks[blockNumbers[i]]
	}

	return protocol.FileInfo{
		Name:     filename,
		Flags:    0,
		Modified: 0,
		Blocks:   existingBlocks,
	}
}

func setUpModel(file protocol.FileInfo) *Model {
	db := db.OpenMemory()
	model := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db, nil)
	model.AddFolder(defaultFolderConfig)
	// Update index
	model.updateLocals("default", []protocol.FileInfo{file})
	return model
}

func setUpRwFolder(model *Model) rwFolder {
	return rwFolder{
		folder:    "default",
		dir:       "testdata",
		model:     model,
		errors:    make(map[string]string),
		errorsMut: sync.NewMutex(),
	}
}

// Layout of the files: (indexes from the above array)
// 12345678 - Required file
// 02005008 - Existing file (currently in the index)
// 02340070 - Temp file on the disk

func TestHandleFile(t *testing.T) {
	// After the diff between required and existing we should:
	// Copy: 2, 5, 8
	// Pull: 1, 3, 4, 6, 7

	existingBlocks := []int{0, 2, 0, 0, 5, 0, 0, 8}
	existingFile := setUpFile("filex", existingBlocks)
	requiredFile := existingFile
	requiredFile.Blocks = blocks[1:]

	m := setUpModel(existingFile)
	p := setUpRwFolder(m)
	copyChan := make(chan copyBlocksState, 1)

	p.handleFile(requiredFile, copyChan, nil)

	// Receive the results
	toCopy := <-copyChan

	if len(toCopy.blocks) != 8 {
		t.Errorf("Unexpected count of copy blocks: %d != 8", len(toCopy.blocks))
	}

	for _, block := range blocks[1:] {
		found := false
		for _, toCopyBlock := range toCopy.blocks {
			if string(toCopyBlock.Hash) == string(block.Hash) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Did not find block %s", block.String())
		}
	}
}

func TestHandleFileWithTemp(t *testing.T) {
	// After diff between required and existing we should:
	// Copy: 2, 5, 8
	// Pull: 1, 3, 4, 6, 7

	// After dropping out blocks already on the temp file we should:
	// Copy: 5, 8
	// Pull: 1, 6

	existingBlocks := []int{0, 2, 0, 0, 5, 0, 0, 8}
	existingFile := setUpFile("file", existingBlocks)
	requiredFile := existingFile
	requiredFile.Blocks = blocks[1:]

	m := setUpModel(existingFile)
	p := setUpRwFolder(m)
	copyChan := make(chan copyBlocksState, 1)

	p.handleFile(requiredFile, copyChan, nil)

	// Receive the results
	toCopy := <-copyChan

	if len(toCopy.blocks) != 4 {
		t.Errorf("Unexpected count of copy blocks: %d != 4", len(toCopy.blocks))
	}

	for _, idx := range []int{1, 5, 6, 8} {
		found := false
		block := blocks[idx]
		for _, toCopyBlock := range toCopy.blocks {
			if string(toCopyBlock.Hash) == string(block.Hash) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Did not find block %s", block.String())
		}
	}
}

func TestCopierFinder(t *testing.T) {
	// After diff between required and existing we should:
	// Copy: 1, 2, 3, 4, 6, 7, 8
	// Since there is no existing file, nor a temp file

	// After dropping out blocks found locally:
	// Pull: 1, 5, 6, 8

	tempFile := filepath.Join("testdata", defTempNamer.TempName("file2"))
	err := os.Remove(tempFile)
	if err != nil && !os.IsNotExist(err) {
		t.Error(err)
	}

	existingBlocks := []int{0, 2, 3, 4, 0, 0, 7, 0}
	existingFile := setUpFile(defTempNamer.TempName("file"), existingBlocks)
	requiredFile := existingFile
	requiredFile.Blocks = blocks[1:]
	requiredFile.Name = "file2"

	m := setUpModel(existingFile)
	p := setUpRwFolder(m)
	copyChan := make(chan copyBlocksState)
	pullChan := make(chan pullBlockState, 4)
	finisherChan := make(chan *sharedPullerState, 1)

	// Run a single fetcher routine
	go p.copierRoutine(copyChan, pullChan, finisherChan)

	p.handleFile(requiredFile, copyChan, finisherChan)

	pulls := []pullBlockState{<-pullChan, <-pullChan, <-pullChan, <-pullChan}
	finish := <-finisherChan

	select {
	case <-pullChan:
		t.Fatal("Finisher channel has data to be read")
	case <-finisherChan:
		t.Fatal("Finisher channel has data to be read")
	default:
	}

	// Verify that the right blocks went into the pull list.
	// They are pulled in random order.
	for _, idx := range []int{1, 5, 6, 8} {
		found := false
		block := blocks[idx]
		for _, pulledBlock := range pulls {
			if string(pulledBlock.block.Hash) == string(block.Hash) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Did not find block %s", block.String())
		}
		if string(finish.file.Blocks[idx-1].Hash) != string(blocks[idx].Hash) {
			t.Errorf("Block %d mismatch: %s != %s", idx, finish.file.Blocks[idx-1].String(), blocks[idx].String())
		}
	}

	// Verify that the fetched blocks have actually been written to the temp file
	blks, err := scanner.HashFile(tempFile, protocol.BlockSize, 0, nil)
	if err != nil {
		t.Log(err)
	}

	for _, eq := range []int{2, 3, 4, 7} {
		if string(blks[eq-1].Hash) != string(blocks[eq].Hash) {
			t.Errorf("Block %d mismatch: %s != %s", eq, blks[eq-1].String(), blocks[eq].String())
		}
	}
	finish.fd.Close()

	os.Remove(tempFile)
}

// Test that updating a file removes it's old blocks from the blockmap
func TestCopierCleanup(t *testing.T) {
	iterFn := func(folder, file string, index int32) bool {
		return true
	}

	// Create a file
	file := setUpFile("test", []int{0})
	m := setUpModel(file)

	file.Blocks = []protocol.BlockInfo{blocks[1]}
	file.Version = file.Version.Update(protocol.LocalDeviceID.Short())
	// Update index (removing old blocks)
	m.updateLocals("default", []protocol.FileInfo{file})

	if m.finder.Iterate(folders, blocks[0].Hash, iterFn) {
		t.Error("Unexpected block found")
	}

	if !m.finder.Iterate(folders, blocks[1].Hash, iterFn) {
		t.Error("Expected block not found")
	}

	file.Blocks = []protocol.BlockInfo{blocks[0]}
	file.Version = file.Version.Update(protocol.LocalDeviceID.Short())
	// Update index (removing old blocks)
	m.updateLocals("default", []protocol.FileInfo{file})

	if !m.finder.Iterate(folders, blocks[0].Hash, iterFn) {
		t.Error("Unexpected block found")
	}

	if m.finder.Iterate(folders, blocks[1].Hash, iterFn) {
		t.Error("Expected block not found")
	}
}

// Make sure that the copier routine hashes the content when asked, and pulls
// if it fails to find the block.
func TestLastResortPulling(t *testing.T) {
	// Add a file to index (with the incorrect block representation, as content
	// doesn't actually match the block list)
	file := setUpFile("empty", []int{0})
	m := setUpModel(file)

	// Pretend that we are handling a new file of the same content but
	// with a different name (causing to copy that particular block)
	file.Name = "newfile"

	iterFn := func(folder, file string, index int32) bool {
		return true
	}

	p := setUpRwFolder(m)

	copyChan := make(chan copyBlocksState)
	pullChan := make(chan pullBlockState, 1)
	finisherChan := make(chan *sharedPullerState, 1)

	// Run a single copier routine
	go p.copierRoutine(copyChan, pullChan, finisherChan)

	p.handleFile(file, copyChan, finisherChan)

	// Copier should hash empty file, realise that the region it has read
	// doesn't match the hash which was advertised by the block map, fix it
	// and ask to pull the block.
	<-pullChan

	// Verify that it did fix the incorrect hash.
	if m.finder.Iterate(folders, blocks[0].Hash, iterFn) {
		t.Error("Found unexpected block")
	}

	if !m.finder.Iterate(folders, scanner.SHA256OfNothing, iterFn) {
		t.Error("Expected block not found")
	}

	(<-finisherChan).fd.Close()
	os.Remove(filepath.Join("testdata", defTempNamer.TempName("newfile")))
}

func TestDeregisterOnFailInCopy(t *testing.T) {
	file := setUpFile("filex", []int{0, 2, 0, 0, 5, 0, 0, 8})
	defer os.Remove("testdata/" + defTempNamer.TempName("filex"))

	db := db.OpenMemory()

	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db, nil)
	m.AddFolder(defaultFolderConfig)

	emitter := NewProgressEmitter(defaultConfig)
	go emitter.Serve()

	p := rwFolder{
		folder:          "default",
		dir:             "testdata",
		model:           m,
		queue:           newJobQueue(),
		progressEmitter: emitter,
		errors:          make(map[string]string),
		errorsMut:       sync.NewMutex(),
	}

	// queue.Done should be called by the finisher routine
	p.queue.Push("filex", 0, 0)
	p.queue.Pop()

	if p.queue.lenProgress() != 1 {
		t.Fatal("Expected file in progress")
	}

	copyChan := make(chan copyBlocksState)
	pullChan := make(chan pullBlockState)
	finisherBufferChan := make(chan *sharedPullerState)
	finisherChan := make(chan *sharedPullerState)

	go p.copierRoutine(copyChan, pullChan, finisherBufferChan)
	go p.finisherRoutine(finisherChan)

	p.handleFile(file, copyChan, finisherChan)

	// Receive a block at puller, to indicate that at least a single copier
	// loop has been performed.
	toPull := <-pullChan
	// Wait until copier is trying to pass something down to the puller again
	time.Sleep(100 * time.Millisecond)
	// Close the file
	toPull.sharedPullerState.fail("test", os.ErrNotExist)
	// Unblock copier
	<-pullChan

	select {
	case state := <-finisherBufferChan:
		// At this point the file should still be registered with both the job
		// queue, and the progress emitter. Verify this.
		if p.progressEmitter.lenRegistry() != 1 || p.queue.lenProgress() != 1 || p.queue.lenQueued() != 0 {
			t.Fatal("Could not find file")
		}

		// Pass the file down the real finisher, and give it time to consume
		finisherChan <- state
		time.Sleep(100 * time.Millisecond)

		state.mut.Lock()
		stateFd := state.fd
		state.mut.Unlock()
		if stateFd != nil {
			t.Fatal("File not closed?")
		}

		if p.progressEmitter.lenRegistry() != 0 || p.queue.lenProgress() != 0 || p.queue.lenQueued() != 0 {
			t.Fatal("Still registered", p.progressEmitter.lenRegistry(), p.queue.lenProgress(), p.queue.lenQueued())
		}

		// Doing it again should have no effect
		finisherChan <- state
		time.Sleep(100 * time.Millisecond)

		if p.progressEmitter.lenRegistry() != 0 || p.queue.lenProgress() != 0 || p.queue.lenQueued() != 0 {
			t.Fatal("Still registered", p.progressEmitter.lenRegistry(), p.queue.lenProgress(), p.queue.lenQueued())
		}
	case <-time.After(time.Second):
		t.Fatal("Didn't get anything to the finisher")
	}
}

func TestDeregisterOnFailInPull(t *testing.T) {
	file := setUpFile("filex", []int{0, 2, 0, 0, 5, 0, 0, 8})
	defer os.Remove("testdata/" + defTempNamer.TempName("filex"))

	db := db.OpenMemory()
	m := NewModel(defaultConfig, protocol.LocalDeviceID, "device", "syncthing", "dev", db, nil)
	m.AddFolder(defaultFolderConfig)

	emitter := NewProgressEmitter(defaultConfig)
	go emitter.Serve()

	p := rwFolder{
		folder:          "default",
		dir:             "testdata",
		model:           m,
		queue:           newJobQueue(),
		progressEmitter: emitter,
		errors:          make(map[string]string),
		errorsMut:       sync.NewMutex(),
	}

	// queue.Done should be called by the finisher routine
	p.queue.Push("filex", 0, 0)
	p.queue.Pop()

	if p.queue.lenProgress() != 1 {
		t.Fatal("Expected file in progress")
	}

	copyChan := make(chan copyBlocksState)
	pullChan := make(chan pullBlockState)
	finisherBufferChan := make(chan *sharedPullerState)
	finisherChan := make(chan *sharedPullerState)

	go p.copierRoutine(copyChan, pullChan, finisherBufferChan)
	go p.pullerRoutine(pullChan, finisherBufferChan)
	go p.finisherRoutine(finisherChan)

	p.handleFile(file, copyChan, finisherChan)

	// Receive at finisher, we should error out as puller has nowhere to pull
	// from.
	select {
	case state := <-finisherBufferChan:
		// At this point the file should still be registered with both the job
		// queue, and the progress emitter. Verify this.
		if p.progressEmitter.lenRegistry() != 1 || p.queue.lenProgress() != 1 || p.queue.lenQueued() != 0 {
			t.Fatal("Could not find file")
		}

		// Pass the file down the real finisher, and give it time to consume
		finisherChan <- state
		time.Sleep(100 * time.Millisecond)

		state.mut.Lock()
		stateFd := state.fd
		state.mut.Unlock()
		if stateFd != nil {
			t.Fatal("File not closed?")
		}

		if p.progressEmitter.lenRegistry() != 0 || p.queue.lenProgress() != 0 || p.queue.lenQueued() != 0 {
			t.Fatal("Still registered", p.progressEmitter.lenRegistry(), p.queue.lenProgress(), p.queue.lenQueued())
		}

		// Doing it again should have no effect
		finisherChan <- state
		time.Sleep(100 * time.Millisecond)

		if p.progressEmitter.lenRegistry() != 0 || p.queue.lenProgress() != 0 || p.queue.lenQueued() != 0 {
			t.Fatal("Still registered", p.progressEmitter.lenRegistry(), p.queue.lenProgress(), p.queue.lenQueued())
		}
	case <-time.After(time.Second):
		t.Fatal("Didn't get anything to the finisher")
	}
}

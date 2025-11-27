// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
)

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

func prepareTmpFile(to fs.Filesystem) (string, error) {
	tmpName := fs.TempName("file")
	in, err := os.Open("testdata/tmpfile")
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := to.Create(tmpName)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err = io.Copy(out, in); err != nil {
		return "", err
	}
	future := time.Now().Add(time.Hour)
	if err := to.Chtimes(tmpName, future, future); err != nil {
		return "", err
	}
	return tmpName, nil
}

var diffTestData = []struct {
	a string
	b string
	s int
	d []protocol.BlockInfo
}{
	{"contents", "contents", 1024, []protocol.BlockInfo{}},
	{"", "", 1024, []protocol.BlockInfo{}},
	{"contents", "contents", 3, []protocol.BlockInfo{}},
	{"contents", "cantents", 3, []protocol.BlockInfo{{Offset: 0, Size: 3}}},
	{"contents", "contants", 3, []protocol.BlockInfo{{Offset: 3, Size: 3}}},
	{"contents", "cantants", 3, []protocol.BlockInfo{{Offset: 0, Size: 3}, {Offset: 3, Size: 3}}},
	{"contents", "", 3, []protocol.BlockInfo{{Offset: 0, Size: 0}}},
	{"", "contents", 3, []protocol.BlockInfo{{Offset: 0, Size: 3}, {Offset: 3, Size: 3}, {Offset: 6, Size: 2}}},
	{"con", "contents", 3, []protocol.BlockInfo{{Offset: 3, Size: 3}, {Offset: 6, Size: 2}}},
	{"contents", "con", 3, nil},
	{"contents", "cont", 3, []protocol.BlockInfo{{Offset: 3, Size: 1}}},
	{"cont", "contents", 3, []protocol.BlockInfo{{Offset: 3, Size: 3}, {Offset: 6, Size: 2}}},
}

func setupFile(filename string, blockNumbers []int) protocol.FileInfo {
	// Create existing file
	existingBlocks := make([]protocol.BlockInfo, len(blockNumbers))
	for i := range blockNumbers {
		existingBlocks[i] = blocks[blockNumbers[i]]
	}

	return protocol.FileInfo{
		Name:   filename,
		Blocks: existingBlocks,
	}
}

func createEmptyFileInfo(t *testing.T, name string, fs fs.Filesystem) protocol.FileInfo {
	t.Helper()

	writeFile(t, fs, name, nil)
	fi, err := fs.Stat(name)
	must(t, err)
	file, err := scanner.CreateFileInfo(fi, name, fs, false, false, config.XattrFilter{})
	must(t, err)
	return file
}

// Sets up a folder and model, but makes sure the services aren't actually running.
func setupSendReceiveFolder(t testing.TB, files ...protocol.FileInfo) (*testModel, *sendReceiveFolder) {
	w, fcfg := newDefaultCfgWrapper(t)
	// Initialise model and stop immediately.
	model := setupModel(t, w)
	model.cancel()
	<-model.stopped
	r, _ := model.folderRunners.Get(fcfg.ID)
	f := r.(*sendReceiveFolder)
	f.tempPullErrors = make(map[string]string)

	// Update index
	if files != nil {
		f.updateLocalsFromScanning(files)
	}

	return model, f
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
	existingFile := setupFile("filex", existingBlocks)
	requiredFile := existingFile
	requiredFile.Blocks = blocks[1:]

	_, f := setupSendReceiveFolder(t, existingFile)

	copyChan := make(chan copyBlocksState, 1)

	f.handleFile(t.Context(), requiredFile, copyChan)

	// Receive the results
	toCopy := <-copyChan

	if len(toCopy.blocks) != 8 {
		t.Errorf("Unexpected count of copy blocks: %d != 8", len(toCopy.blocks))
	}

	for _, block := range blocks[1:] {
		found := false
		for _, toCopyBlock := range toCopy.blocks {
			if bytes.Equal(toCopyBlock.Hash, block.Hash) {
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
	existingFile := setupFile("file", existingBlocks)
	requiredFile := existingFile
	requiredFile.Blocks = blocks[1:]

	_, f := setupSendReceiveFolder(t, existingFile)

	if _, err := prepareTmpFile(f.Filesystem()); err != nil {
		t.Fatal(err)
	}

	copyChan := make(chan copyBlocksState, 1)

	f.handleFile(t.Context(), requiredFile, copyChan)

	// Receive the results
	toCopy := <-copyChan

	if len(toCopy.blocks) != 4 {
		t.Errorf("Unexpected count of copy blocks: %d != 4", len(toCopy.blocks))
	}

	for _, idx := range []int{1, 5, 6, 8} {
		found := false
		block := blocks[idx]
		for _, toCopyBlock := range toCopy.blocks {
			if bytes.Equal(toCopyBlock.Hash, block.Hash) {
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

	tempFile := fs.TempName("file2")

	existingBlocks := []int{0, 2, 3, 4, 0, 0, 7, 0}
	existingFile := setupFile(fs.TempName("file"), existingBlocks)
	existingFile.Size = 1
	requiredFile := existingFile
	requiredFile.Blocks = blocks[1:]
	requiredFile.Name = "file2"

	_, f := setupSendReceiveFolder(t, existingFile)

	if _, err := prepareTmpFile(f.Filesystem()); err != nil {
		t.Fatal(err)
	}

	copyChan := make(chan copyBlocksState)
	pullChan := make(chan pullBlockState, 4)
	finisherChan := make(chan *sharedPullerState, 1)

	// Run a single fetcher routine
	go f.copierRoutine(t.Context(), copyChan, pullChan, finisherChan)
	defer close(copyChan)

	f.handleFile(t.Context(), requiredFile, copyChan)

	timeout := time.After(10 * time.Second)
	pulls := make([]pullBlockState, 4)
	for i := 0; i < 4; i++ {
		select {
		case pulls[i] = <-pullChan:
		case <-timeout:
			t.Fatalf("Timed out before receiving all 4 states on pullChan (already got %v)", i)
		}
	}
	var finish *sharedPullerState
	select {
	case finish = <-finisherChan:
	case <-timeout:
		t.Fatal("Timed out before receiving 4 states on pullChan")
	}

	defer cleanupSharedPullerState(finish)

	select {
	case v := <-pullChan:
		t.Logf("%+v\n", v)
		t.Fatal("Pull channel had data to be read")
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
			if bytes.Equal(pulledBlock.block.Hash, block.Hash) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Did not find block %s", block.String())
		}
		if !bytes.Equal(finish.file.Blocks[idx-1].Hash, blocks[idx].Hash) {
			t.Errorf("Block %d mismatch: %s != %s", idx, finish.file.Blocks[idx-1].String(), blocks[idx].String())
		}
	}

	// Verify that the fetched blocks have actually been written to the temp file
	blks, err := scanner.HashFile(t.Context(), f.ID, f.Filesystem(), tempFile, protocol.MinBlockSize, nil)
	if err != nil {
		t.Log(err)
	}

	for _, eq := range []int{2, 3, 4, 7} {
		if !bytes.Equal(blks[eq-1].Hash, blocks[eq].Hash) {
			t.Errorf("Block %d mismatch: %s != %s", eq, blks[eq-1].String(), blocks[eq].String())
		}
	}
}

// Test that updating a file removes its old blocks from the blockmap
func TestCopierCleanup(t *testing.T) {
	// Create a file
	file := setupFile("test", []int{0})
	file.Size = 1
	m, f := setupSendReceiveFolder(t, file)

	file.Blocks = []protocol.BlockInfo{blocks[1]}
	file.Version = file.Version.Update(myID.Short())
	// Update index (removing old blocks)
	f.updateLocalsFromScanning([]protocol.FileInfo{file})

	if vals, err := itererr.Collect(m.sdb.AllLocalBlocksWithHash(f.ID, blocks[0].Hash)); err != nil || len(vals) > 0 {
		t.Error("Unexpected block found")
	}

	if vals, err := itererr.Collect(m.sdb.AllLocalBlocksWithHash(f.ID, blocks[1].Hash)); err != nil || len(vals) == 0 {
		t.Error("Expected block not found")
	}

	file.Blocks = []protocol.BlockInfo{blocks[0]}
	file.Version = file.Version.Update(myID.Short())
	// Update index (removing old blocks)
	f.updateLocalsFromScanning([]protocol.FileInfo{file})

	if vals, err := itererr.Collect(m.sdb.AllLocalBlocksWithHash(f.ID, blocks[0].Hash)); err != nil || len(vals) == 0 {
		t.Error("Unexpected block found")
	}

	if vals, err := itererr.Collect(m.sdb.AllLocalBlocksWithHash(f.ID, blocks[1].Hash)); err != nil || len(vals) > 0 {
		t.Error("Expected block not found")
	}
}

func TestDeregisterOnFailInCopy(t *testing.T) {
	file := setupFile("filex", []int{0, 2, 0, 0, 5, 0, 0, 8})

	m, f := setupSendReceiveFolder(t)

	// Set up our evet subscription early
	s := m.evLogger.Subscribe(events.ItemFinished)

	// queue.Done should be called by the finisher routine
	f.queue.Push("filex", 0, time.Time{})
	f.queue.Pop()

	if f.queue.lenProgress() != 1 {
		t.Fatal("Expected file in progress")
	}

	pullChan := make(chan pullBlockState)
	finisherBufferChan := make(chan *sharedPullerState, 1)
	finisherChan := make(chan *sharedPullerState)
	dbUpdateChan := make(chan dbUpdateJob, 1)

	copyChan, copyWg := startCopier(t.Context(), f, pullChan, finisherBufferChan)
	go f.finisherRoutine(t.Context(), finisherChan, dbUpdateChan, make(chan string))

	defer func() {
		close(copyChan)
		copyWg.Wait()
		close(pullChan)
		close(finisherBufferChan)
		close(finisherChan)
	}()

	f.handleFile(t.Context(), file, copyChan)

	// Receive a block at puller, to indicate that at least a single copier
	// loop has been performed.
	var toPull pullBlockState
	select {
	case toPull = <-pullChan:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}

	// Unblock copier
	go func() {
		for range pullChan {
		}
	}()

	// Close the file, causing errors on further access
	toPull.sharedPullerState.fail(os.ErrNotExist)

	select {
	case state := <-finisherBufferChan:
		// At this point the file should still be registered with both the job
		// queue, and the progress emitter. Verify this.
		if f.model.progressEmitter.lenRegistry() != 1 || f.queue.lenProgress() != 1 || f.queue.lenQueued() != 0 {
			t.Fatal("Could not find file")
		}

		// Pass the file down the real finisher, and give it time to consume
		finisherChan <- state

		t0 := time.Now()
		if ev, err := s.Poll(time.Minute); err != nil {
			t.Fatal("Got error waiting for ItemFinished event:", err)
		} else if n := ev.Data.(map[string]interface{})["item"]; n != state.file.Name {
			t.Fatal("Got ItemFinished event for wrong file:", n)
		}
		t.Log("event took", time.Since(t0))

		state.mut.Lock()
		stateWriter := state.writer
		state.mut.Unlock()
		if stateWriter != nil {
			t.Fatal("File not closed?")
		}

		if f.model.progressEmitter.lenRegistry() != 0 || f.queue.lenProgress() != 0 || f.queue.lenQueued() != 0 {
			t.Fatal("Still registered", f.model.progressEmitter.lenRegistry(), f.queue.lenProgress(), f.queue.lenQueued())
		}

		// Doing it again should have no effect
		finisherChan <- state

		if _, err := s.Poll(time.Second); err != events.ErrTimeout {
			t.Fatal("Expected timeout, not another event", err)
		}

		if f.model.progressEmitter.lenRegistry() != 0 || f.queue.lenProgress() != 0 || f.queue.lenQueued() != 0 {
			t.Fatal("Still registered", f.model.progressEmitter.lenRegistry(), f.queue.lenProgress(), f.queue.lenQueued())
		}

	case <-time.After(5 * time.Second):
		t.Fatal("Didn't get anything to the finisher")
	}
}

func TestDeregisterOnFailInPull(t *testing.T) {
	file := setupFile("filex", []int{0, 2, 0, 0, 5, 0, 0, 8})

	m, f := setupSendReceiveFolder(t)

	// Set up our evet subscription early
	s := m.evLogger.Subscribe(events.ItemFinished)

	// queue.Done should be called by the finisher routine
	f.queue.Push("filex", 0, time.Time{})
	f.queue.Pop()

	if f.queue.lenProgress() != 1 {
		t.Fatal("Expected file in progress")
	}

	pullChan := make(chan pullBlockState)
	finisherBufferChan := make(chan *sharedPullerState)
	finisherChan := make(chan *sharedPullerState)
	dbUpdateChan := make(chan dbUpdateJob, 1)

	copyChan, copyWg := startCopier(t.Context(), f, pullChan, finisherBufferChan)
	var pullWg sync.WaitGroup
	pullWg.Add(1)
	go func() {
		f.pullerRoutine(t.Context(), pullChan, finisherBufferChan)
		pullWg.Done()
	}()
	go f.finisherRoutine(t.Context(), finisherChan, dbUpdateChan, make(chan string))
	defer func() {
		// Unblock copier and puller
		go func() {
			for range finisherBufferChan {
			}
		}()
		close(copyChan)
		copyWg.Wait()
		close(pullChan)
		pullWg.Wait()
		close(finisherBufferChan)
		close(finisherChan)
	}()

	f.handleFile(t.Context(), file, copyChan)

	// Receive at finisher, we should error out as puller has nowhere to pull
	// from.
	timeout = time.Second

	// Both the puller and copier may send to the finisherBufferChan.
	var state *sharedPullerState
	after := time.After(5 * time.Second)
	for {
		select {
		case state = <-finisherBufferChan:
		case <-after:
			t.Fatal("Didn't get failed state to the finisher")
		}
		if state.failed() != nil {
			break
		}
	}

	// At this point the file should still be registered with both the job
	// queue, and the progress emitter. Verify this.
	if f.model.progressEmitter.lenRegistry() != 1 || f.queue.lenProgress() != 1 || f.queue.lenQueued() != 0 {
		t.Fatal("Could not find file")
	}

	// Pass the file down the real finisher, and give it time to consume
	finisherChan <- state

	t0 := time.Now()
	if ev, err := s.Poll(time.Minute); err != nil {
		t.Fatal("Got error waiting for ItemFinished event:", err)
	} else if n := ev.Data.(map[string]interface{})["item"]; n != state.file.Name {
		t.Fatal("Got ItemFinished event for wrong file:", n)
	}
	t.Log("event took", time.Since(t0))

	state.mut.Lock()
	stateWriter := state.writer
	state.mut.Unlock()
	if stateWriter != nil {
		t.Fatal("File not closed?")
	}

	if f.model.progressEmitter.lenRegistry() != 0 || f.queue.lenProgress() != 0 || f.queue.lenQueued() != 0 {
		t.Fatal("Still registered", f.model.progressEmitter.lenRegistry(), f.queue.lenProgress(), f.queue.lenQueued())
	}

	// Doing it again should have no effect
	finisherChan <- state

	if _, err := s.Poll(time.Second); err != events.ErrTimeout {
		t.Fatal("Expected timeout, not another event", err)
	}

	if f.model.progressEmitter.lenRegistry() != 0 || f.queue.lenProgress() != 0 || f.queue.lenQueued() != 0 {
		t.Fatal("Still registered", f.model.progressEmitter.lenRegistry(), f.queue.lenProgress(), f.queue.lenQueued())
	}
}

func TestIssue3164(t *testing.T) {
	_, f := setupSendReceiveFolder(t)
	ffs := f.Filesystem()

	ignDir := filepath.Join("issue3164", "oktodelete")
	subDir := filepath.Join(ignDir, "foobar")
	must(t, ffs.MkdirAll(subDir, 0o777))
	must(t, fs.WriteFile(ffs, filepath.Join(subDir, "file"), []byte("Hello"), 0o644))
	must(t, fs.WriteFile(ffs, filepath.Join(ignDir, "file"), []byte("Hello"), 0o644))
	file := protocol.FileInfo{
		Name: "issue3164",
	}

	must(t, f.scanSubdirs(t.Context(), nil))

	matcher := ignore.New(ffs)
	must(t, matcher.Parse(bytes.NewBufferString("(?d)oktodelete"), ""))
	f.ignores = matcher

	dbUpdateChan := make(chan dbUpdateJob, 1)

	f.deleteDir(file, dbUpdateChan, make(chan string))

	if _, err := ffs.Stat("issue3164"); !fs.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestDiff(t *testing.T) {
	for i, test := range diffTestData {
		a, _ := scanner.Blocks(t.Context(), bytes.NewBufferString(test.a), test.s, -1, nil)
		b, _ := scanner.Blocks(t.Context(), bytes.NewBufferString(test.b), test.s, -1, nil)
		_, d := blockDiff(a, b)
		if len(d) != len(test.d) {
			t.Fatalf("Incorrect length for diff %d; %d != %d", i, len(d), len(test.d))
		} else {
			for j := range test.d {
				if d[j].Offset != test.d[j].Offset {
					t.Errorf("Incorrect offset for diff %d block %d; %d != %d", i, j, d[j].Offset, test.d[j].Offset)
				}
				if d[j].Size != test.d[j].Size {
					t.Errorf("Incorrect length for diff %d block %d; %d != %d", i, j, d[j].Size, test.d[j].Size)
				}
			}
		}
	}
}

func BenchmarkDiff(b *testing.B) {
	testCases := make([]struct{ a, b []protocol.BlockInfo }, 0, len(diffTestData))
	for _, test := range diffTestData {
		aBlocks, _ := scanner.Blocks(b.Context(), bytes.NewBufferString(test.a), test.s, -1, nil)
		bBlocks, _ := scanner.Blocks(b.Context(), bytes.NewBufferString(test.b), test.s, -1, nil)
		testCases = append(testCases, struct{ a, b []protocol.BlockInfo }{aBlocks, bBlocks})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tc := range testCases {
			blockDiff(tc.a, tc.b)
		}
	}
}

func TestDiffEmpty(t *testing.T) {
	emptyCases := []struct {
		a    []protocol.BlockInfo
		b    []protocol.BlockInfo
		need int
		have int
	}{
		{nil, nil, 0, 0},
		{[]protocol.BlockInfo{{Offset: 3, Size: 1}}, nil, 0, 0},
		{nil, []protocol.BlockInfo{{Offset: 3, Size: 1}}, 1, 0},
	}
	for _, emptyCase := range emptyCases {
		h, n := blockDiff(emptyCase.a, emptyCase.b)
		if len(h) != emptyCase.have {
			t.Errorf("incorrect have: %d != %d", len(h), emptyCase.have)
		}
		if len(n) != emptyCase.need {
			t.Errorf("incorrect have: %d != %d", len(h), emptyCase.have)
		}
	}
}

// TestDeleteIgnorePerms checks, that a file gets deleted when the IgnorePerms
// option is true and the permissions do not match between the file on disk and
// in the db.
func TestDeleteIgnorePerms(t *testing.T) {
	_, f := setupSendReceiveFolder(t)
	ffs := f.Filesystem()
	f.IgnorePerms = true

	name := "deleteIgnorePerms"
	file, err := ffs.Create(name)
	if err != nil {
		t.Error(err)
	}
	defer file.Close()

	stat, err := file.Stat()
	must(t, err)
	fi, err := scanner.CreateFileInfo(stat, name, ffs, false, false, config.XattrFilter{})
	must(t, err)
	ffs.Chmod(name, 0o600)
	if info, err := ffs.Stat(name); err == nil {
		fi.InodeChangeNs = info.InodeChangeTime().UnixNano()
	}
	scanChan := make(chan string, 1)
	err = f.checkToBeDeleted(fi, fi, true, scanChan)
	must(t, err)
}

func TestCopyOwner(t *testing.T) {
	// Verifies that owner and group are copied from the parent, for both
	// files and directories.

	if build.IsWindows {
		t.Skip("copying owner not supported on Windows")
	}

	const (
		expOwner = 1234
		expGroup = 5678
	)

	// This test hung on a regression, taking a long time to fail - speed that up.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	go func() {
		<-ctx.Done()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			pprof.Lookup("goroutine").WriteTo(os.Stdout, 2)
			panic("timed out before test finished")
		}
	}()

	// Set up a folder with the CopyParentOwner bit and backed by a fake
	// filesystem.

	m, f := setupSendReceiveFolder(t)
	f.folder.FolderConfiguration = newFolderConfiguration(m.cfg, f.ID, f.Label, config.FilesystemTypeFake, "/TestCopyOwner")
	f.folder.FolderConfiguration.CopyOwnershipFromParent = true

	// Create a parent dir with a certain owner/group.

	f.mtimefs.Mkdir("foo", 0o755)
	f.mtimefs.Lchown("foo", strconv.Itoa(expOwner), strconv.Itoa(expGroup))

	dir := protocol.FileInfo{
		Name:        "foo/bar",
		Type:        protocol.FileInfoTypeDirectory,
		Permissions: 0o755,
	}

	// Have the folder create a subdirectory, verify that it's the correct
	// owner/group.

	dbUpdateChan := make(chan dbUpdateJob, 1)
	scanChan := make(chan string)
	defer close(dbUpdateChan)
	f.handleDir(dir, dbUpdateChan, scanChan)
	select {
	case <-dbUpdateChan: // empty the channel for later
	case toScan := <-scanChan:
		t.Fatal("Unexpected receive on scanChan:", toScan)
	}

	info, err := f.mtimefs.Lstat("foo/bar")
	if err != nil {
		t.Fatal("Unexpected error (dir):", err)
	}
	if info.Owner() != expOwner || info.Group() != expGroup {
		t.Fatalf("Expected dir owner/group to be %d/%d, not %d/%d", expOwner, expGroup, info.Owner(), info.Group())
	}

	// Have the folder create a file, verify it's the correct owner/group.
	// File is zero sized to avoid having to handle copies/pulls.

	file := protocol.FileInfo{
		Name:        "foo/bar/baz",
		Type:        protocol.FileInfoTypeFile,
		Permissions: 0o644,
	}

	// Wire some stuff. The flow here is handleFile() -[copierChan]->
	// copierRoutine() -[finisherChan]-> finisherRoutine() -[dbUpdateChan]->
	// back to us and we're done. The copier routine doesn't do anything,
	// but it's the way data is passed around. When the database update
	// comes the finisher is done.

	finisherChan := make(chan *sharedPullerState)
	copierChan, copyWg := startCopier(t.Context(), f, nil, finisherChan)
	go f.finisherRoutine(t.Context(), finisherChan, dbUpdateChan, nil)
	defer func() {
		close(copierChan)
		copyWg.Wait()
		close(finisherChan)
	}()

	f.handleFile(t.Context(), file, copierChan)
	<-dbUpdateChan

	info, err = f.mtimefs.Lstat("foo/bar/baz")
	if err != nil {
		t.Fatal("Unexpected error (file):", err)
	}
	if info.Owner() != expOwner || info.Group() != expGroup {
		t.Fatalf("Expected file owner/group to be %d/%d, not %d/%d", expOwner, expGroup, info.Owner(), info.Group())
	}

	// Have the folder create a symlink. Verify it accordingly.
	symlink := protocol.FileInfo{
		Name:          "foo/bar/sym",
		Type:          protocol.FileInfoTypeSymlink,
		Permissions:   0o644,
		SymlinkTarget: []byte("over the rainbow"),
	}

	f.handleSymlink(symlink, dbUpdateChan, scanChan)
	select {
	case <-dbUpdateChan:
	case toScan := <-scanChan:
		t.Fatal("Unexpected receive on scanChan:", toScan)
	}

	info, err = f.mtimefs.Lstat("foo/bar/sym")
	if err != nil {
		t.Fatal("Unexpected error (file):", err)
	}
	if info.Owner() != expOwner || info.Group() != expGroup {
		t.Fatalf("Expected symlink owner/group to be %d/%d, not %d/%d", expOwner, expGroup, info.Owner(), info.Group())
	}
}

// TestSRConflictReplaceFileByDir checks that a conflict is created when an existing file
// is replaced with a directory and versions are conflicting
func TestSRConflictReplaceFileByDir(t *testing.T) {
	_, f := setupSendReceiveFolder(t)
	ffs := f.Filesystem()

	name := "foo"

	// create local file
	file := createEmptyFileInfo(t, name, ffs)
	file.Version = protocol.Vector{}.Update(myID.Short())
	f.updateLocalsFromScanning([]protocol.FileInfo{file})

	// Simulate remote creating a dir with the same name
	file.Type = protocol.FileInfoTypeDirectory
	rem := device1.Short()
	file.Version = protocol.Vector{}.Update(rem)
	file.ModifiedBy = rem

	dbUpdateChan := make(chan dbUpdateJob, 1)
	scanChan := make(chan string, 1)

	f.handleDir(file, dbUpdateChan, scanChan)

	if confls := existingConflicts(name, ffs); len(confls) != 1 {
		t.Fatal("Expected one conflict, got", len(confls))
	} else if scan := <-scanChan; confls[0] != scan {
		t.Fatal("Expected request to scan", confls[0], "got", scan)
	}
}

// TestSRConflictReplaceFileByLink checks that a conflict is created when an existing file
// is replaced with a link and versions are conflicting
func TestSRConflictReplaceFileByLink(t *testing.T) {
	_, f := setupSendReceiveFolder(t)
	ffs := f.Filesystem()

	name := "foo"

	// create local file
	file := createEmptyFileInfo(t, name, ffs)
	file.Version = protocol.Vector{}.Update(myID.Short())
	f.updateLocalsFromScanning([]protocol.FileInfo{file})

	// Simulate remote creating a symlink with the same name
	file.Type = protocol.FileInfoTypeSymlink
	file.SymlinkTarget = []byte("bar")
	rem := device1.Short()
	file.Version = protocol.Vector{}.Update(rem)
	file.ModifiedBy = rem

	dbUpdateChan := make(chan dbUpdateJob, 1)
	scanChan := make(chan string, 1)

	f.handleSymlink(file, dbUpdateChan, scanChan)

	if confls := existingConflicts(name, ffs); len(confls) != 1 {
		t.Fatal("Expected one conflict, got", len(confls))
	} else if scan := <-scanChan; confls[0] != scan {
		t.Fatal("Expected request to scan", confls[0], "got", scan)
	}
}

// TestDeleteBehindSymlink checks that we don't delete or schedule a scan
// when trying to delete a file behind a symlink.
func TestDeleteBehindSymlink(t *testing.T) {
	_, f := setupSendReceiveFolder(t)
	ffs := f.Filesystem()

	link := "link"
	linkFile := filepath.Join(link, "file")

	must(t, ffs.MkdirAll(link, 0o755))
	fi := createEmptyFileInfo(t, linkFile, ffs)
	f.updateLocalsFromScanning([]protocol.FileInfo{fi})
	must(t, ffs.Rename(linkFile, "file"))
	must(t, ffs.RemoveAll(link))
	must(t, ffs.CreateSymlink("/", link))

	fi.Deleted = true
	fi.Version = fi.Version.Update(device1.Short())
	scanChan := make(chan string, 1)
	dbUpdateChan := make(chan dbUpdateJob, 1)
	f.deleteFile(fi, dbUpdateChan, scanChan)
	select {
	case f := <-scanChan:
		t.Fatalf("Received %v on scanChan", f)
	case u := <-dbUpdateChan:
		if u.jobType != dbUpdateDeleteFile {
			t.Errorf("Expected jobType %v, got %v", dbUpdateDeleteFile, u.jobType)
		}
		if u.file.Name != fi.Name {
			t.Errorf("Expected update for %v, got %v", fi.Name, u.file.Name)
		}
	default:
		t.Fatalf("No db update received")
	}
	if _, err := ffs.Stat("file"); err != nil {
		t.Errorf("Expected no error when stating file behind symlink, got %v", err)
	}
}

// Reproduces https://github.com/syncthing/syncthing/issues/6559
func TestPullCtxCancel(t *testing.T) {
	_, f := setupSendReceiveFolder(t)

	pullChan := make(chan pullBlockState)
	finisherChan := make(chan *sharedPullerState)

	ctx, cancel := context.WithCancel(t.Context())

	go f.pullerRoutine(ctx, pullChan, finisherChan)
	defer close(pullChan)

	emptyState := func() pullBlockState {
		return pullBlockState{
			sharedPullerState: newSharedPullerState(protocol.FileInfo{}, nil, f.folderID, "", nil, nil, false, false, protocol.FileInfo{}, false, false),
			block:             protocol.BlockInfo{},
		}
	}

	cancel()

	done := make(chan struct{})
	defer close(done)
	for i := 0; i < 2; i++ {
		go func() {
			select {
			case pullChan <- emptyState():
			case <-done:
			}
		}()
		select {
		case s := <-finisherChan:
			if s.failed() == nil {
				t.Errorf("state %v not failed", i)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out before receiving state %v on finisherChan", i)
		}
	}
}

func TestPullDeleteUnscannedDir(t *testing.T) {
	_, f := setupSendReceiveFolder(t)
	ffs := f.Filesystem()

	dir := "foobar"
	must(t, ffs.MkdirAll(dir, 0o777))
	fi := protocol.FileInfo{
		Name: dir,
	}

	scanChan := make(chan string, 1)
	dbUpdateChan := make(chan dbUpdateJob, 1)

	f.deleteDir(fi, dbUpdateChan, scanChan)

	if _, err := ffs.Stat(dir); fs.IsNotExist(err) {
		t.Error("directory has been deleted")
	}
	select {
	case toScan := <-scanChan:
		if toScan != dir {
			t.Errorf("expected %v to be scanned, got %v", dir, toScan)
		}
	default:
		t.Error("nothing was scheduled for scanning")
	}
}

func TestPullCaseOnlyPerformFinish(t *testing.T) {
	m, f := setupSendReceiveFolder(t)
	ffs := f.Filesystem()

	name := "foo"
	contents := []byte("contents")
	writeFile(t, ffs, name, contents)
	must(t, f.scanSubdirs(t.Context(), nil))

	var cur protocol.FileInfo
	hasCur := false
	it, errFn := m.LocalFiles(f.ID, protocol.LocalDeviceID)
	for i := range it {
		if hasCur {
			t.Fatal("got more than one file")
		}
		cur = i
		hasCur = true
	}
	if err := errFn(); err != nil {
		t.Fatal(err)
	}
	if !hasCur {
		t.Fatal("file is missing")
	}

	remote := cur
	remote.Version = protocol.Vector{}.Update(device1.Short())
	remote.Name = strings.ToUpper(cur.Name)

	temp := fs.TempName(remote.Name)
	writeFile(t, ffs, temp, contents)
	scanChan := make(chan string, 1)
	dbUpdateChan := make(chan dbUpdateJob, 1)

	err := f.performFinish(remote, cur, hasCur, temp, dbUpdateChan, scanChan)

	select {
	case <-dbUpdateChan: // boring case sensitive filesystem
		return
	case <-scanChan:
		t.Error("no need to scan anything here")
	default:
	}

	var caseErr *fs.CaseConflictError
	if !errors.As(err, &caseErr) {
		t.Error("Expected case conflict error, got", err)
	}
}

func TestPullCaseOnlyDir(t *testing.T) {
	testPullCaseOnlyDirOrSymlink(t, true)
}

func TestPullCaseOnlySymlink(t *testing.T) {
	if build.IsWindows {
		t.Skip("symlinks not supported on windows")
	}
	testPullCaseOnlyDirOrSymlink(t, false)
}

func testPullCaseOnlyDirOrSymlink(t *testing.T, dir bool) {
	m, f := setupSendReceiveFolder(t)
	ffs := f.Filesystem()

	name := "foo"
	if dir {
		must(t, ffs.Mkdir(name, 0o777))
	} else {
		must(t, ffs.CreateSymlink("target", name))
	}

	must(t, f.scanSubdirs(t.Context(), nil))
	var cur protocol.FileInfo
	hasCur := false
	it, errFn := m.LocalFiles(f.ID, protocol.LocalDeviceID)
	for i := range it {
		if hasCur {
			t.Fatal("got more than one file")
		}
		cur = i
		hasCur = true
	}
	if err := errFn(); err != nil {
		t.Fatal(err)
	}
	if !hasCur {
		t.Fatal("file is missing")
	}

	scanChan := make(chan string, 1)
	dbUpdateChan := make(chan dbUpdateJob, 1)

	remote := cur
	remote.Version = protocol.Vector{}.Update(device1.Short())
	remote.Name = strings.ToUpper(cur.Name)

	if dir {
		f.handleDir(remote, dbUpdateChan, scanChan)
	} else {
		f.handleSymlink(remote, dbUpdateChan, scanChan)
	}

	select {
	case <-dbUpdateChan: // boring case sensitive filesystem
		return
	case <-scanChan:
		t.Error("no need to scan anything here")
	default:
	}
	if errStr, ok := f.tempPullErrors[remote.Name]; !ok {
		t.Error("missing error for", remote.Name)
	} else if !strings.Contains(errStr, "uses different upper or lowercase") {
		t.Error("unexpected error", errStr, "for", remote.Name)
	}
}

func TestPullTempFileCaseConflict(t *testing.T) {
	_, f := setupSendReceiveFolder(t)

	copyChan := make(chan copyBlocksState, 1)

	file := protocol.FileInfo{Name: "foo"}
	confl := "Foo"
	tempNameConfl := fs.TempName(confl)
	if fd, err := f.mtimefs.Create(tempNameConfl); err != nil {
		t.Fatal(err)
	} else {
		if _, err := fd.Write([]byte("data")); err != nil {
			t.Fatal(err)
		}
		fd.Close()
	}

	f.handleFile(t.Context(), file, copyChan)

	cs := <-copyChan
	if _, err := cs.tempFile(); err != nil {
		t.Error(err)
	} else {
		cs.finalClose()
	}
}

func TestPullCaseOnlyRename(t *testing.T) {
	m, f := setupSendReceiveFolder(t)

	// tempNameConfl := fs.TempName(confl)

	name := "foo"
	if fd, err := f.mtimefs.Create(name); err != nil {
		t.Fatal(err)
	} else {
		if _, err := fd.Write([]byte("data")); err != nil {
			t.Fatal(err)
		}
		fd.Close()
	}

	must(t, f.scanSubdirs(t.Context(), nil))

	cur, ok := m.testCurrentFolderFile(f.ID, name)
	if !ok {
		t.Fatal("file missing")
	}

	deleted := cur
	deleted.SetDeleted(myID.Short())

	confl := cur
	confl.Name = "Foo"
	confl.Version = confl.Version.Update(device1.Short())

	dbUpdateChan := make(chan dbUpdateJob, 2)
	scanChan := make(chan string, 2)
	if err := f.renameFile(cur, deleted, confl, dbUpdateChan, scanChan); err != nil {
		t.Error(err)
	}
}

func TestPullSymlinkOverExistingWindows(t *testing.T) {
	if !build.IsWindows {
		t.Skip()
	}

	m, f := setupSendReceiveFolder(t)
	conn := addFakeConn(m, device1, f.ID)

	name := "foo"
	if fd, err := f.mtimefs.Create(name); err != nil {
		t.Fatal(err)
	} else {
		if _, err := fd.Write([]byte("data")); err != nil {
			t.Fatal(err)
		}
		fd.Close()
	}

	must(t, f.scanSubdirs(t.Context(), nil))

	file, ok := m.testCurrentFolderFile(f.ID, name)
	if !ok {
		t.Fatal("file missing")
	}
	must(t, m.Index(conn, &protocol.Index{Folder: f.ID, Files: []protocol.FileInfo{{Name: name, Type: protocol.FileInfoTypeSymlink, Version: file.Version.Update(device1.Short())}}}))

	scanChan := make(chan string)

	changed, err := f.pullerIteration(t.Context(), scanChan)
	must(t, err)
	if changed != 1 {
		t.Error("Expected one change in pull, got", changed)
	}
	if file, ok := m.testCurrentFolderFile(f.ID, name); !ok {
		t.Error("symlink entry missing")
	} else if !file.IsUnsupported() {
		t.Error("symlink entry isn't marked as unsupported")
	}
	if _, err := f.mtimefs.Lstat(name); err == nil {
		t.Error("old file still exists on disk")
	} else if !fs.IsNotExist(err) {
		t.Error(err)
	}
}

func TestPullDeleteCaseConflict(t *testing.T) {
	_, f := setupSendReceiveFolder(t)

	name := "foo"
	fi := protocol.FileInfo{Name: "Foo"}
	dbUpdateChan := make(chan dbUpdateJob, 1)
	scanChan := make(chan string)

	if fd, err := f.mtimefs.Create(name); err != nil {
		t.Fatal(err)
	} else {
		if _, err := fd.Write([]byte("data")); err != nil {
			t.Fatal(err)
		}
		fd.Close()
	}
	f.deleteFileWithCurrent(fi, protocol.FileInfo{}, false, dbUpdateChan, scanChan)
	select {
	case <-dbUpdateChan:
	default:
		t.Error("Missing db update for file")
	}

	f.deleteDir(fi, dbUpdateChan, scanChan)
	select {
	case <-dbUpdateChan:
	default:
		t.Error("Missing db update for dir")
	}
}

func TestPullDeleteIgnoreChildDir(t *testing.T) {
	_, f := setupSendReceiveFolder(t)

	parent := "parent"
	del := "ignored"
	child := "keep"
	matcher := ignore.New(f.mtimefs)
	must(t, matcher.Parse(bytes.NewBufferString(fmt.Sprintf(`
!%v
(?d)%v
`, child, del)), ""))
	f.ignores = matcher

	must(t, f.mtimefs.Mkdir(parent, 0o777))
	must(t, f.mtimefs.Mkdir(filepath.Join(parent, del), 0o777))
	must(t, f.mtimefs.Mkdir(filepath.Join(parent, del, child), 0o777))

	scanChan := make(chan string, 2)

	err := f.deleteDirOnDisk(parent, scanChan)
	if err == nil {
		t.Error("no error")
	}
}

func cleanupSharedPullerState(s *sharedPullerState) {
	s.mut.Lock()
	defer s.mut.Unlock()
	if s.writer == nil {
		return
	}
	s.writer.mut.Lock()
	s.writer.fd.Close()
	s.writer.mut.Unlock()
}

func startCopier(ctx context.Context, f *sendReceiveFolder, pullChan chan<- pullBlockState, finisherChan chan<- *sharedPullerState) (chan copyBlocksState, *sync.WaitGroup) {
	copyChan := make(chan copyBlocksState)
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		f.copierRoutine(ctx, copyChan, pullChan, finisherChan)
		wg.Done()
	}()
	return copyChan, wg
}

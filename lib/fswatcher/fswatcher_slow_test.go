// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package fswatcher

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func init() {
	if err := os.RemoveAll(testDir); err != nil {
		panic(err)
	}
}

var (
	testDir      = "temporary_test_fswatcher"
	notifyDelayS = 1
)

// TestTemplate illustrates how a test can be created.
// It also checkes some basic operations like file creation, deletion, renaming
// and folder creation and behaviour like reactivating timer on new event
func TestTemplate(t *testing.T) {
	// create dirs/files that should exist before FS watching
	oldfile := createTestFile(t, "oldfile")

	// dir/file manipulations during FS watching
	file1 := "file1"
	file2 := "dir1/file2"
	dir1 := "dir1"
	newfile := "newfile"
	testCase := func() {
		// test timer reactivation
		sleepMs(1000)
		createTestFile(t, file1)
		createTestDir(t, dir1)
		sleepMs(1100)
		renameTestFile(t, oldfile, newfile)
		createTestFile(t, file2)
		sleepMs(1000)
		deleteTestFile(t, file1)
		deleteTestDir(t, dir1)
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		expectedBatch{[]string{file1, dir1}, 2000, 2500},
		expectedBatch{[]string{file2, oldfile, newfile}, 3000, 3500},
		expectedBatch{[]string{file1, dir1}, 4000, 4500},
	}

	testScenario(t, "Template", testCase, expectedBatches)
}

// TestAggregate checks whether maxFilesPerDir+10 events in one dir are
// aggregated to parent dir
func TestAggregate(t *testing.T) {
	parent := createTestDir(t, "parent")
	var files [maxFilesPerDir + 10]string
	for i := 0; i < maxFilesPerDir+10; i++ {
		files[i] = filepath.Join(parent, strconv.Itoa(i))
	}
	testCase := func() {
		for _, file := range files {
			createTestFile(t, file)
		}
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		expectedBatch{[]string{parent}, 1000, 1500},
	}

	testScenario(t, "Aggregate", testCase, expectedBatches)
}

// TestAggregateParent checks whether maxFilesPerDir events in one dir and
// event in a subdir of dir are aggregated
func TestAggregateParent(t *testing.T) {
	parent := createTestDir(t, "parent")
	createTestDir(t, filepath.Join(parent, "dir"))
	var files [maxFilesPerDir]string
	for i := 0; i < maxFilesPerDir; i++ {
		files[i] = filepath.Join(parent, strconv.Itoa(i))
	}
	childFile := "parent/dir/childFile"
	testCase := func() {
		for _, file := range files {
			createTestFile(t, file)
		}
		sleepMs(50)
		createTestFile(t, childFile)
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		expectedBatch{[]string{parent}, 1000, 1500},
	}

	testScenario(t, "AggregateParent", testCase, expectedBatches)
}

// TestRootAggreagate checks that maxFiles+10 events in root dir are aggregated
func TestRootAggregate(t *testing.T) {
	var files [maxFiles + 10]string
	for i := 0; i < maxFiles+10; i++ {
		files[i] = strconv.Itoa(i)
	}
	testCase := func() {
		for _, file := range files {
			createTestFile(t, file)
		}
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		expectedBatch{[]string{"."}, 1000, 1500},
	}

	testScenario(t, "RootAggregate", testCase, expectedBatches)
}

// TestRootNotAggreagate checks that maxFilesPerDir+10 events in root dir are
// not aggregated
func TestRootNotAggregate(t *testing.T) {
	var files [maxFilesPerDir + 10]string
	for i := 0; i < maxFilesPerDir+10; i++ {
		files[i] = strconv.Itoa(i)
	}
	testCase := func() {
		for _, file := range files {
			createTestFile(t, file)
		}
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		expectedBatch{files[:], 1000, 1500},
	}

	testScenario(t, "RootNotAggregate", testCase, expectedBatches)
}

// TestDelay checks recurring changes to the same path delays sending it
func TestDelay(t *testing.T) {
	file := createTestFile(t, "file")
	testCase := func() {
		writeTestFile(t, file, "first")
		for i := 0; i < 15; i++ {
			sleepMs(400)
			writeTestFile(t, file, strconv.Itoa(i))
		}
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		expectedBatch{[]string{file}, 5900, 6500},
		expectedBatch{[]string{file}, 6900, 7500},
	}

	testScenario(t, "Delay", testCase, expectedBatches)
}

// TestOverflow checks that the entire folder is scanned when maxFiles is reached
func TestOverflow(t *testing.T) {
	var dirs [maxFiles/100 + 1]string
	for i := 0; i < maxFiles/100+1; i++ {
		dirs[i] = createTestDir(t, "dir"+strconv.Itoa(i))
	}
	testCase := func() {
		for _, dir := range dirs {
			for i := 0; i < 100; i++ {
				createTestFile(t, filepath.Join(dir,
					"file"+strconv.Itoa(i)))
			}
		}
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		expectedBatch{[]string{"."}, 1000, 1500},
	}

	testScenario(t, "Overflow", testCase, expectedBatches)
}

// TestOutside checks that no changes from outside the folder make it in
func TestOutside(t *testing.T) {
	dir := createTestDir(t, "dir")
	outDir := "temp-outside"
	if err := os.RemoveAll(outDir); err != nil {
		panic(err)
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		panic(fmt.Sprintf("Failed to create directory %s: %s", outDir,
			err))
	}
	createTestFile(t, "dir/file")
	testCase := func() {
		if err := os.Rename(filepath.Join(testDir, dir),
			filepath.Join(outDir, dir)); err != nil {
			panic(err)
		}
		if err := os.RemoveAll(outDir); err != nil {
			panic(err)
		}
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		expectedBatch{[]string{dir}, 1000, 1500},
	}

	testScenario(t, "Outside", testCase, expectedBatches)
}

func testFsWatcher(t *testing.T, name string) Service {
	dir, err := filepath.Abs(".")
	if err != nil {
		panic("Cannot get absolute path to working dir")
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		panic("Cannot get real path to working dir")
	}
	watcher, err := NewFsWatcher(filepath.Join(dir, testDir), name, nil, notifyDelayS)
	if err != nil {
		t.Errorf("Starting FS notifications failed: %s", err)
		return nil
	}
	return watcher
}

// path relative to folder root
func writeTestFile(t *testing.T, path string, text string) {
	if err := ioutil.WriteFile(filepath.Join(testDir, path), []byte(text),
		0664); err != nil {
		panic(fmt.Sprintf("Failed to write to file %s: %s", path, err))
	}
}

// path relative to folder root
func renameTestFile(t *testing.T, old string, new string) {
	if err := os.Rename(filepath.Join(testDir, old),
		filepath.Join(testDir, new)); err != nil {
		panic(fmt.Sprintf("Failed to rename %s to %s: %s", old, new,
			err))
	}
}

// path relative to folder root
func deleteTestFile(t *testing.T, file string) {
	if err := os.Remove(filepath.Join(testDir, file)); err != nil {
		panic(fmt.Sprintf("Failed to delete %s: %s", file, err))
	}
}

// path relative to folder root
func deleteTestDir(t *testing.T, dir string) {
	if err := os.RemoveAll(filepath.Join(testDir, dir)); err != nil {
		panic(fmt.Sprintf("Failed to delete %s: %s", dir, err))
	}
}

// path relative to folder root, also creates parent dirs if necessary
func createTestFile(t *testing.T, file string) string {
	if err := os.MkdirAll(filepath.Dir(filepath.Join(testDir, file)), 0755); err != nil {
		panic(fmt.Sprintf("Failed to parent directory for %s: %s", file,
			err))
	}
	handle, err := os.Create(filepath.Join(testDir, file))
	if err != nil {
		panic(fmt.Sprintf("Failed to create test file %s: %s", file, err))
	}
	handle.Close()
	return file
}

// path relative to folder root
func createTestDir(t *testing.T, dir string) string {
	if err := os.MkdirAll(filepath.Join(testDir, dir), 0755); err != nil {
		panic(fmt.Sprintf("Failed to create directory %s: %s", dir, err))
	}
	return dir
}

func sleepMs(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

func durationMs(ms int) time.Duration {
	return time.Duration(ms) * time.Millisecond
}

func testSliceInBatchKeys(t *testing.T, batch FsEventsBatch, paths []string, batchIndex int) {
	pathSet := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		pathSet[path] = struct{}{}
		if _, ok := batch[path]; ok {
			delete(batch, path)
		} else {
			t.Errorf("Did not receive event %s in batch %d", path,
				batchIndex+1)
		}
	}
	for path := range batch {
		if _, ok := pathSet[path]; ok {
			t.Errorf("Received unexpected event %s in batch %d",
				path, batchIndex+1)
		}
	}
}

type expectedBatch struct {
	paths    []string
	afterMs  int
	beforeMs int
}

func testScenario(t *testing.T, name string, testCase func(),
	expectedBatches []expectedBatch) {
	createTestDir(t, ".")

	fsWatcher := testFsWatcher(t, name)

	abort := make(chan struct{})

	startTime := time.Now()

	go fsWatcher.Serve()

	// To allow using round numbers in expected times
	sleepMs(10)
	go testFsWatcherOutput(t, fsWatcher.FsWatchChan(), expectedBatches, startTime, abort)

	testCase()
	sleepMs(1100)

	abort <- struct{}{}
	fsWatcher.Stop()
	os.RemoveAll(testDir)
	// Without delay events kind of randomly creep over to next test
	// number is magic by trial (100 wasn't enough)
	sleepMs(500)
}

func testFsWatcherOutput(t *testing.T, fsWatchChan <-chan FsEventsBatch,
	expectedBatches []expectedBatch, startTime time.Time, abort <-chan struct{}) {
	var received FsEventsBatch
	var elapsedTime time.Duration
	batchIndex := 0
	for {
		select {
		case <-abort:
			if batchIndex != len(expectedBatches) {
				t.Errorf("Received only %d batches (%d expected)",
					batchIndex, len(expectedBatches))
			}
			return
		case received = <-fsWatchChan:
		}

		if batchIndex >= len(expectedBatches) {
			t.Errorf("Received batch %d (only %d expected)",
				batchIndex+1, len(expectedBatches))
			continue
		}

		elapsedTime = time.Since(startTime)
		expected := expectedBatches[batchIndex]
		switch {
		case elapsedTime < durationMs(expected.afterMs):
			t.Errorf("Received batch %d after %v (too soon)",
				batchIndex+1, elapsedTime)

		case elapsedTime > durationMs(expected.beforeMs):
			t.Errorf("Received batch %d after %v (too late)",
				batchIndex+1, elapsedTime)

		case len(received) != len(expected.paths):
			t.Errorf("Received %v events instead of %v for batch %v",
				len(received), len(expected.paths), batchIndex+1)
		}
		testSliceInBatchKeys(t, received, expected.paths, batchIndex)
		batchIndex++
	}
}

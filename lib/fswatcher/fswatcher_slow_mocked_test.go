// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package fswatcher

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/zillode/notify"
)

const (
	folderRoot   = "/home/someuser/syncthing"
)

// TestTemplate illustrates how a test can be created.
// It also checkes some basic operations like file creation, deletion, renaming
// and folder creation and behaviour like reactivating timer on new event
func TestTemplateMockedBackend(t *testing.T) {
	// simulated dir/file manipulations
	file1 := "file1"
	file2 := "dir1/file2"
	dir1 := "dir1"
	oldfile := "oldfile"
	newfile := "newfile"
	testCase := func(c chan<- notify.EventInfo) {
		// test timer reactivation
		sleepMs(1000)
		sendEvent(t, c, file1)
		sendEvent(t, c, dir1)
		sleepMs(1100)
		// represents renaming just without backend so boring...
		sendEvent(t, c, oldfile)
		sendEvent(t, c, newfile)
		sendEvent(t, c, file2)
		sleepMs(1000)
		// represents deleting, just without backend so boring...
		sendEvent(t, c, file1)
		sendEvent(t, c, dir1)
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		expectedBatch{[]string{file1, dir1}, 2000, 2500},
		expectedBatch{[]string{file2, oldfile, newfile}, 3000, 3500},
		expectedBatch{[]string{file1, dir1}, 4000, 4500},
	}

	testScenarioMocked(t, "Template", testCase, expectedBatches)
}

// TestAggregate checks whether maxFilesPerDir+10 events in one dir are
// aggregated to parent dir
func TestAggregateMockedBackend(t *testing.T) {
	parent := "parent"
	var files [maxFilesPerDir + 10]string
	for i := 0; i < maxFilesPerDir+10; i++ {
		files[i] = filepath.Join(parent, strconv.Itoa(i))
	}
	testCase := func(c chan<- notify.EventInfo) {
		for _, file := range files {
			sendEvent(t, c, file)
		}
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		expectedBatch{[]string{parent}, 1000, 1500},
	}

	testScenarioMocked(t, "Aggregate", testCase, expectedBatches)
}

// TestAggregateParent checks whether maxFilesPerDir events in one dir and
// event in a subdir of dir are aggregated
func TestAggregateParentMockedBackend(t *testing.T) {
	parent := "parent"
	var files [maxFilesPerDir]string
	for i := 0; i < maxFilesPerDir; i++ {
		files[i] = filepath.Join(parent, strconv.Itoa(i))
	}
	childFile := "parent/dir/childFile"
	testCase := func(c chan<- notify.EventInfo) {
		for _, file := range files {
			sendEvent(t, c, file)
		}
		sleepMs(50)
		sendEvent(t, c, childFile)
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		expectedBatch{[]string{parent}, 1000, 1500},
	}

	testScenarioMocked(t, "AggregateParent", testCase, expectedBatches)
}

// TestRootAggreagate checks that maxFiles+10 events in root dir are aggregated
func TestRootAggregateMockedBackend(t *testing.T) {
	var files [maxFiles + 10]string
	for i := 0; i < maxFiles+10; i++ {
		files[i] = strconv.Itoa(i)
	}
	testCase := func(c chan<- notify.EventInfo) {
		for _, file := range files {
			sendEvent(t, c, file)
		}
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		expectedBatch{[]string{"."}, 1000, 1500},
	}

	testScenarioMocked(t, "RootAggregate", testCase, expectedBatches)
}

// TestRootNotAggreagate checks that maxFilesPerDir+10 events in root dir are
// not aggregated
func TestRootNotAggregateMockedBackend(t *testing.T) {
	var files [maxFilesPerDir + 10]string
	for i := 0; i < maxFilesPerDir+10; i++ {
		files[i] = strconv.Itoa(i)
	}
	testCase := func(c chan<- notify.EventInfo) {
		for _, file := range files {
			sendEvent(t, c, file)
		}
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		expectedBatch{files[:], 1000, 1500},
	}

	testScenarioMocked(t, "RootNotAggregate", testCase, expectedBatches)
}

// TestDelay checks recurring changes to the same path delays sending it
func TestDelayMockedBackend(t *testing.T) {
	file := "file"
	testCase := func(c chan<- notify.EventInfo) {
		for i := 0; i < 12; i++ {
			sleepMs(500)
			sendEvent(t, c, file)
		}
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		expectedBatch{[]string{file}, 5900, 6500},
		expectedBatch{[]string{file}, 6900, 7500},
	}

	testScenarioMocked(t, "Delay", testCase, expectedBatches)
}

// TestOverflow checks that the entire folder is scanned when maxFiles is reached
func TestOverflowMockedBackend(t *testing.T) {
	var dirs [maxFiles/100 + 1]string
	for i := 0; i < maxFiles/100+1; i++ {
		dirs[i] = "dir"+strconv.Itoa(i)
	}
	testCase := func(c chan<- notify.EventInfo) {
		for _, dir := range dirs {
			for i := 0; i < 100; i++ {
				sendEvent(t, c, filepath.Join(dir,
					"file"+strconv.Itoa(i)))
			}
		}
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		expectedBatch{[]string{"."}, 1000, 1500},
	}

	testScenarioMocked(t, "Overflow", testCase, expectedBatches)
}

// TestOutside checks that no changes from outside the folder make it in
func TestOutsideMockedBackend(t *testing.T) {
	dir := "dir"
	testCase := func(c chan<- notify.EventInfo) {
		sendAbsEvent(t, c, filepath.Join(filepath.Dir(folderRoot), dir))
	}

	expectedBatches := []expectedBatch{}

	testScenarioMocked(t, "Outside", testCase, expectedBatches)
}

func testScenarioMocked(t *testing.T, name string,
	testCase func(chan<- notify.EventInfo), expectedBatches []expectedBatch) {
	fsWatcher := NewFsWatcher(folderRoot, name, nil, notifyDelayS)

	fsEventChan := make(chan notify.EventInfo, maxFiles)
	abort := make(chan struct{})

	notifyModelChan := make(chan FsEventsBatch)
	fsWatcher.notifyModelChan = notifyModelChan
	fsWatcher.fsEventChan = fsEventChan
	fsWatcher.WatchingFs = true

	startTime := time.Now()
	go fsWatcher.watchFilesystem()

	// To allow using round numbers in expected times
	sleepMs(10)
	go testFsWatcherOutput(t, fsWatcher, notifyModelChan, expectedBatches,
		startTime, abort)

	testCase(fsEventChan)
	sleepMs(1100)

	abort <- struct{}{}
	fsWatcher.Stop()
	// Without delay events kind of randomly creep over to next test
	// number is magic by trial (100 wasn't enough)
	// sleepMs(500)
}

type fakeEventInfo string

func (e fakeEventInfo) Path() string {
	return string(e)
}

func (e fakeEventInfo) Event() notify.Event {
	return 0
}

func (e fakeEventInfo) Sys() interface{} {
	return nil
}

func sendEvent(t *testing.T, c chan<- notify.EventInfo, path string) {
	sendAbsEvent(t, c, filepath.Join(folderRoot, path))
}

func sendAbsEvent(t *testing.T, c chan<- notify.EventInfo, path string) {
	timer := time.NewTimer(durationMs(50))
	// var event fakeEventInfo = filepath.Join(folderRoot, path)
	select {
	case c <- fakeEventInfo(path):
	// case c <- event:
	case <- timer.C:
		t.Errorf("Sending blocked longer than 10ms (real backend drops immediately)")
	}
	timer.Stop()
}



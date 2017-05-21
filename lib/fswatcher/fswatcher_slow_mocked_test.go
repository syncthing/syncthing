// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build !solaris,!darwin solaris,cgo darwin,cgo

package fswatcher

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/zillode/notify"
)

var folderRoot = filepath.Clean("/home/someuser/syncthing")

// TestAggregate checks whether maxFilesPerDir+1 events in one dir are
// aggregated to parent dir
func TestAggregateMockedBackend(t *testing.T) {
	parent := "parent"
	files := make([]string, maxFilesPerDir+1)
	for i := 0; i < maxFilesPerDir+1; i++ {
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
	files := make([]string, maxFilesPerDir)
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

// TestRootAggreagate checks that maxFiles+1 events in root dir are aggregated
func TestRootAggregateMockedBackend(t *testing.T) {
	files := make([]string, maxFiles+1)
	for i := 0; i < maxFiles+1; i++ {
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

// TestRootNotAggreagate checks that maxFilesPerDir+1 events in root dir are
// not aggregated
func TestRootNotAggregateMockedBackend(t *testing.T) {
	files := make([]string, maxFilesPerDir+1)
	for i := 0; i < maxFilesPerDir+1; i++ {
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
		delay := time.Duration(300) * time.Millisecond
		timer := time.NewTimer(delay)
		for i := 0; i < 14; i++ {
			<-timer.C
			timer.Reset(delay)
			sendEvent(t, c, file)
		}
		<-timer.C
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		expectedBatch{[]string{file}, 3900, 4500},
		expectedBatch{[]string{file}, 4900, 5500},
	}

	testScenarioMocked(t, "Delay", testCase, expectedBatches)
}

// TestOverflow checks that the entire folder is scanned when maxFiles is reached
func TestOverflowMockedBackend(t *testing.T) {
	filesPerDir := maxFiles / 5
	dirs := make([]string, maxFiles/filesPerDir+1)
	for i := 0; i < maxFiles/filesPerDir+1; i++ {
		dirs[i] = "dir" + strconv.Itoa(i)
	}
	testCase := func(c chan<- notify.EventInfo) {
		for _, dir := range dirs {
			for i := 0; i < filesPerDir; i++ {
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

// TestChannelOverflow tries to overflow the event input channel (inherently racy)
func TestChannelOverflowMockedBackend(t *testing.T) {
	testCase := func(c chan<- notify.EventInfo) {
		for i := 0; i < 2*maxFiles; i++ {
			sendEventImmediately(t, c, "file"+strconv.Itoa(i))
		}
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		expectedBatch{[]string{"."}, 1000, 1500},
	}

	testScenarioMocked(t, "ChannelOverflow", testCase, expectedBatches)
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
	fsWatcher := &fsWatcher{
		folderPath:            folderRoot,
		notifyModelChan:       make(chan []string),
		rootEventDir:          newEventDir(".", nil),
		fsEventChan:           make(chan notify.EventInfo, maxFiles),
		notifyDelay:           time.Duration(notifyDelayS) * time.Second,
		notifyTimeout:         testNotifyTimeout,
		notifyTimerNeedsReset: false,
		inProgress:            make(map[string]struct{}),
		description:           name + "Mocked",
		ignores:               nil,
		ignoresUpdate:         nil,
		resetNotifyTimerChan:  make(chan time.Duration),
		stop:                  make(chan struct{}),
	}

	abort := make(chan struct{})

	startTime := time.Now()
	go fsWatcher.Serve()

	// To allow using round numbers in expected times
	sleepMs(10)
	go testFsWatcherOutput(t, fsWatcher.notifyModelChan, expectedBatches, startTime, abort)

	testCase(fsWatcher.fsEventChan)
	sleepMs(1100)

	abort <- struct{}{}
	fsWatcher.Stop()
	<-abort
}

type fakeEventInfo string

func (e fakeEventInfo) Path() string {
	return string(e)
}

func (e fakeEventInfo) Event() notify.Event {
	return notify.Write
}

func (e fakeEventInfo) Sys() interface{} {
	return nil
}

func sendEvent(t *testing.T, c chan<- notify.EventInfo, path string) {
	sendAbsEvent(t, c, filepath.Join(folderRoot, path))
}

func sendEventImmediately(t *testing.T, c chan<- notify.EventInfo, path string) {
	sendAbsEventTimed(t, c, filepath.Join(folderRoot, path), time.Duration(0))
}

func sendAbsEvent(t *testing.T, c chan<- notify.EventInfo, path string) {
	sendAbsEventTimed(t, c, path, time.Microsecond)
}

func sendAbsEventTimed(t *testing.T, c chan<- notify.EventInfo, path string, delay time.Duration) {
	// This simulates the time the actual backend takes between sending
	// events (exact delay is pure guesswork)
	time.Sleep(delay)
	select {
	case c <- fakeEventInfo(path):
	default:
		// real backend drops events immediately on blocking channel
	}
}

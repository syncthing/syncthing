// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package watchaggregator

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/fs"
)

var folderRoot = filepath.Clean("/home/someuser/syncthing")

// TestDelayMockedBackend checks recurring changes to the same path delays sending it
func TestDelayMockedBackend(t *testing.T) {
	file := "file"
	testCase := func(c chan<- fs.Event) {
		sleepMs(200)
		delay := time.Duration(300) * time.Millisecond
		timer := time.NewTimer(delay)
		for i := 0; i < 10; i++ {
			<-timer.C
			timer.Reset(delay)
			sendEvent(t, c, file)
		}
		<-timer.C
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		{[]string{file}, 2500, 4500},
		{[]string{file}, 3600, 6500},
	}

	testScenarioMocked(t, "Delay", testCase, expectedBatches)
}

// TestChannelOverflow tries to overflow the event input channel (inherently racy)
func TestChannelOverflowMockedBackend(t *testing.T) {
	testCase := func(c chan<- fs.Event) {
		for i := 0; i < 2*maxFiles; i++ {
			sendEventImmediately(t, c, "file"+strconv.Itoa(i))
		}
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		{[]string{"."}, 900, 1600},
	}

	testScenarioMocked(t, "ChannelOverflow", testCase, expectedBatches)
}

func testScenarioMocked(t *testing.T, name string, testCase func(chan<- fs.Event), expectedBatches []expectedBatch) {
	name = name + "-mocked"

	folderCfg := config.FolderConfiguration{
		ID:              name,
		FilesystemType:  fs.FilesystemTypeBasic,
		Path:            folderRoot,
		FSWatcherDelayS: testNotifyDelayS,
	}
	cfg := config.Configuration{
		Folders: []config.FolderConfiguration{folderCfg},
	}
	wrapper := config.Wrap("", cfg)

	ctx, cancel := context.WithCancel(context.Background())
	a := new(folderCfg, ctx)
	a.notifyTimeout = testNotifyTimeout

	abort := make(chan struct{})
	eventChan := make(chan fs.Event, maxFiles)
	watchChan := make(chan []string)

	startTime := time.Now()
	go a.mainLoop(eventChan, watchChan, wrapper)

	// To allow using round numbers in expected times
	sleepMs(10)
	go testFsWatcherOutput(t, watchChan, expectedBatches, startTime, abort)

	timeout := time.NewTimer(time.Duration(expectedBatches[len(expectedBatches)-1].beforeMs+100) * time.Millisecond)
	testCase(eventChan)
	<-timeout.C

	abort <- struct{}{}
	cancel()
	<-abort
}

func sendEvent(t *testing.T, c chan<- fs.Event, path string) {
	sendEventTimed(t, c, path, time.Microsecond)
}

func sendEventImmediately(t *testing.T, c chan<- fs.Event, path string) {
	sendEventTimed(t, c, path, time.Duration(0))
}

func sendEventTimed(t *testing.T, c chan<- fs.Event, path string, delay time.Duration) {
	// This simulates the time the actual backend takes between sending
	// events (exact delay is pure guesswork)
	time.Sleep(delay)
	select {
	case c <- fs.Event{Name: path, Type: fs.NonRemove}:
	default:
		// real backend drops events immediately on blocking channel
	}
}

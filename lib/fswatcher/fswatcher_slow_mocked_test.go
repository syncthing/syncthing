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

	"github.com/syncthing/syncthing/lib/config"
	"github.com/zillode/notify"
)

var folderRoot = filepath.Clean("/home/someuser/syncthing")

// TestDelayMockedBackend checks recurring changes to the same path delays sending it
func TestDelayMockedBackend(t *testing.T) {
	file := "file"
	testCase := func(c chan<- notify.EventInfo) {
		sleepMs(200)
		delay := time.Duration(300) * time.Millisecond
		timer := time.NewTimer(delay)
		for i := 0; i < 13; i++ {
			<-timer.C
			timer.Reset(delay)
			sendEvent(t, c, file)
		}
		<-timer.C
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		expectedBatch{[]string{file}, 3900, 5500},
		expectedBatch{[]string{file}, 4900, 7000},
	}

	testScenarioMocked(t, "Delay", testCase, expectedBatches)
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
		expectedBatch{[]string{"."}, 900, 1500},
	}

	testScenarioMocked(t, "ChannelOverflow", testCase, expectedBatches)
}

func testScenarioMocked(t *testing.T, name string, testCase func(chan<- notify.EventInfo), expectedBatches []expectedBatch) {
	name = name + "-mocked"
	folderCfg := config.FolderConfiguration{
		ID:                    name,
		RawPath:               folderRoot,
		FsNotificationsDelayS: testNotifyDelayS,
	}
	cfg := config.Configuration{
		Folders: []config.FolderConfiguration{folderCfg},
	}
	wrapper := config.Wrap("", cfg)
	fsWatcher := &fsWatcher{
		folderID:              name,
		notifyModelChan:       make(chan []string),
		rootEventDir:          newEventDir(".", nil),
		fsEventChan:           make(chan notify.EventInfo, maxFiles),
		notifyTimerNeedsReset: false,
		inProgress:            make(map[string]struct{}),
		ignores:               nil,
		ignoresUpdate:         nil,
		resetNotifyTimerChan:  make(chan time.Duration),
		stop:                  make(chan struct{}),
		cfg:                   wrapper,
	}
	fsWatcher.updateConfig(folderCfg)
	fsWatcher.notifyTimeout = testNotifyTimeout

	folderRoot = fsWatcher.folderPath

	abort := make(chan struct{})

	startTime := time.Now()
	go fsWatcher.Serve()

	// To allow using round numbers in expected times
	sleepMs(10)
	go testFsWatcherOutput(t, fsWatcher.notifyModelChan, expectedBatches, startTime, abort)

	timeout := time.NewTimer(time.Duration(expectedBatches[len(expectedBatches)-1].beforeMs+100) * time.Millisecond)
	testCase(fsWatcher.fsEventChan)
	<-timeout.C

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

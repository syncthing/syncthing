// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build !solaris,!darwin solaris,cgo darwin,cgo

package fs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/zillode/notify"
)

func TestMain(m *testing.M) {
	if err := os.RemoveAll(testDir); err != nil {
		panic(err)
	}

	dir, err := filepath.Abs(".")
	if err != nil {
		panic("Cannot get absolute path to working dir")
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		panic("Cannot get real path to working dir")
	}
	testDirAbs = filepath.Join(dir, testDir)
	testFs = newBasicFilesystem(testDirAbs)
	if l.ShouldDebug("filesystem") {
		testFs = &logFilesystem{testFs}
	}

	backendBuffer = 10
	defer func() {
		backendBuffer = 500
	}()
	os.Exit(m.Run())
}

const (
	testDir = "temporary_test_root"
)

var (
	testDirAbs string
	testFs     Filesystem
)

func TestWatchIgnore(t *testing.T) {
	file := "file"
	ignored := "ignored"

	testCase := func() {
		createTestFile(t, file)
		createTestFile(t, ignored)
	}

	expectedEvents := []Event{
		{file, NonRemove},
	}

	testScenario(t, "Ignore", testCase, expectedEvents, false, ignored)
}

func TestWatchRename(t *testing.T) {
	old := createTestFile(t, "oldfile")
	new := "newfile"

	testCase := func() {
		if err := testFs.Rename(old, new); err != nil {
			panic(fmt.Sprintf("Failed to rename %s to %s: %s", old, new, err))
		}
	}

	destEvent := Event{new, Remove}
	// Only on these platforms the removed file can be differentiated from
	// the created file during renaming
	if runtime.GOOS == "windows" || runtime.GOOS == "linux" || runtime.GOOS == "solaris" {
		destEvent = Event{new, NonRemove}
	}
	expectedEvents := []Event{
		{old, Remove},
		destEvent,
	}

	testScenario(t, "Rename", testCase, expectedEvents, false, "")
}

// TestWatchOutside checks that no changes from outside the folder make it in
func TestWatchOutside(t *testing.T) {
	outChan := make(chan Event)
	backendChan := make(chan notify.EventInfo, backendBuffer)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer func() {
			if recover() == nil {
				t.Fatalf("Watch did not panic on receiving event outside of folder")
			}
			cancel()
		}()
		testFs.(*BasicFilesystem).watchLoop(testDirAbs, backendChan, outChan, fakeMatcher{}, ctx)
	}()

	backendChan <- fakeEventInfo(filepath.Join(filepath.Dir(testDirAbs), "outside"))
}

// TestWatchOverflow checks that an event at the root is sent when maxFiles is reached
func TestWatchOverflow(t *testing.T) {
	testCase := func() {
		for i := 0; i < 5*backendBuffer; i++ {
			createTestFile(t, "file"+strconv.Itoa(i))
		}
	}

	expectedEvents := []Event{
		{".", NonRemove},
	}

	testScenario(t, "Overflow", testCase, expectedEvents, true, "")
}

func createTestDir(t *testing.T, dir string) string {
	if err := testFs.MkdirAll(dir, 0755); err != nil {
		panic(fmt.Sprintf("Failed to create directory %s: %s", dir, err))
	}
	return dir
}

// path relative to folder root, also creates parent dirs if necessary
func createTestFile(t *testing.T, file string) string {
	if err := testFs.MkdirAll(filepath.Dir(file), 0755); err != nil {
		panic(fmt.Sprintf("Failed to create parent directory for %s: %s", file, err))
	}
	handle, err := testFs.Create(file)
	if err != nil {
		panic(fmt.Sprintf("Failed to create test file %s: %s", file, err))
	}
	handle.Close()
	return file
}

func sleepMs(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

func testScenario(t *testing.T, name string, testCase func(), expectedEvents []Event, allowOthers bool, ignored string) {
	createTestDir(t, ".")

	// Tests pick up the previously created files/dirs, probably because
	// they get flushed to disk with a delay.
	initDelayMs := 500
	if runtime.GOOS == "darwin" {
		initDelayMs = 900
	}
	sleepMs(initDelayMs)

	ctx, cancel := context.WithCancel(context.Background())

	eventChan, err := testFs.Watch(".", fakeMatcher{ignored}, ctx, false)
	if err != nil {
		panic(err)
	}

	go testWatchOutput(t, eventChan, expectedEvents, allowOthers, ctx, cancel)

	timeout := time.NewTimer(2 * time.Second)

	testCase()

	select {
	case <-timeout.C:
		t.Errorf("Timed out before receiving all expected events")
		cancel()
	case <-ctx.Done():
	}

	os.RemoveAll(testDir)

	// Without delay, tests fail with spurious error on windows on file
	// operations in successive tests
	if runtime.GOOS == "windows" {
		sleepMs(500)
	}
}

func testWatchOutput(t *testing.T, in <-chan Event, expectedEvents []Event, allowOthers bool, ctx context.Context, cancel context.CancelFunc) {
	var expected = make(map[Event]struct{})
	for _, ev := range expectedEvents {
		expected[ev] = struct{}{}
	}

	var received Event
	var last Event
	for {
		if len(expected) == 0 {
			cancel()
			return
		}

		select {
		case <-ctx.Done():
			return
		case received = <-in:
		}

		// apparently the backend sometimes sends repeat events
		if last == received {
			continue
		}

		if _, ok := expected[received]; !ok {
			if allowOthers {
				sleepMs(100) // To facilitate overflow
				continue
			}
			t.Errorf("Received unexpected event %v expected one of %v", received, expected)
			cancel()
			return
		}
		delete(expected, received)
		last = received
	}
}

type fakeMatcher struct{ match string }

func (fm fakeMatcher) ShouldIgnore(name string) bool {
	return name == fm.match
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

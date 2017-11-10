// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package watchaggregator

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
)

func TestMain(m *testing.M) {
	maxFiles = 32
	maxFilesPerDir = 8
	defer func() {
		maxFiles = 512
		maxFilesPerDir = 128
	}()

	os.Exit(m.Run())
}

const (
	testNotifyDelayS  = 1
	testNotifyTimeout = 2 * time.Second
)

var (
	folderRoot       = filepath.Clean("/home/someuser/syncthing")
	defaultFolderCfg = config.FolderConfiguration{
		FilesystemType:  fs.FilesystemTypeBasic,
		Path:            folderRoot,
		FSWatcherDelayS: testNotifyDelayS,
	}
	defaultCfg = config.Wrap("", config.Configuration{
		Folders: []config.FolderConfiguration{defaultFolderCfg},
	})
)

type expectedBatch struct {
	paths    []string
	afterMs  int
	beforeMs int
}

// TestAggregate checks whether maxFilesPerDir+1 events in one dir are
// aggregated to parent dir
func TestAggregate(t *testing.T) {
	evDir := newEventDir()
	inProgress := make(map[string]struct{})

	folderCfg := defaultFolderCfg.Copy()
	folderCfg.ID = "Aggregate"
	ctx, _ := context.WithCancel(context.Background())
	a := newAggregator(folderCfg, ctx)

	// checks whether maxFilesPerDir events in one dir are kept as is
	for i := 0; i < maxFilesPerDir; i++ {
		a.newEvent(fs.Event{filepath.Join("parent", strconv.Itoa(i)), fs.NonRemove}, evDir, inProgress)
	}
	if len(getEventPaths(evDir, ".", a)) != maxFilesPerDir {
		t.Errorf("Unexpected number of events stored")
	}

	// checks whether maxFilesPerDir+1 events in one dir are aggregated to parent dir
	a.newEvent(fs.Event{filepath.Join("parent", "new"), fs.NonRemove}, evDir, inProgress)
	compareBatchToExpected(t, getEventPaths(evDir, ".", a), []string{"parent"})

	// checks that adding an event below "parent" does not change anything
	a.newEvent(fs.Event{filepath.Join("parent", "extra"), fs.NonRemove}, evDir, inProgress)
	compareBatchToExpected(t, getEventPaths(evDir, ".", a), []string{"parent"})

	// again test aggregation in "parent" but with event in subdirs
	evDir = newEventDir()
	for i := 0; i < maxFilesPerDir; i++ {
		a.newEvent(fs.Event{filepath.Join("parent", strconv.Itoa(i)), fs.NonRemove}, evDir, inProgress)
	}
	a.newEvent(fs.Event{filepath.Join("parent", "sub", "new"), fs.NonRemove}, evDir, inProgress)
	compareBatchToExpected(t, getEventPaths(evDir, ".", a), []string{"parent"})

	// test aggregation in root
	evDir = newEventDir()
	for i := 0; i < maxFiles; i++ {
		a.newEvent(fs.Event{strconv.Itoa(i), fs.NonRemove}, evDir, inProgress)
	}
	if len(getEventPaths(evDir, ".", a)) != maxFiles {
		t.Errorf("Unexpected number of events stored in root")
	}
	a.newEvent(fs.Event{filepath.Join("parent", "sub", "new"), fs.NonRemove}, evDir, inProgress)
	compareBatchToExpected(t, getEventPaths(evDir, ".", a), []string{"."})

	// checks that adding an event when "." is already stored is a noop
	a.newEvent(fs.Event{"anythingelse", fs.NonRemove}, evDir, inProgress)
	compareBatchToExpected(t, getEventPaths(evDir, ".", a), []string{"."})

	// TestOverflow checks that the entire folder is scanned when maxFiles is reached
	evDir = newEventDir()
	filesPerDir := maxFilesPerDir / 2
	dirs := make([]string, maxFiles/filesPerDir+1)
	for i := 0; i < maxFiles/filesPerDir+1; i++ {
		dirs[i] = "dir" + strconv.Itoa(i)
	}
	for _, dir := range dirs {
		for i := 0; i < filesPerDir; i++ {
			a.newEvent(fs.Event{filepath.Join(dir, strconv.Itoa(i)), fs.NonRemove}, evDir, inProgress)
		}
	}
	compareBatchToExpected(t, getEventPaths(evDir, ".", a), []string{"."})
}

// TestInProgress checks that ignoring files currently edited by Syncthing works
func TestInProgress(t *testing.T) {
	testCase := func(c chan<- fs.Event) {
		events.Default.Log(events.ItemStarted, map[string]string{
			"item": "inprogress",
		})
		sleepMs(100)
		c <- fs.Event{Name: "inprogress", Type: fs.NonRemove}
		sleepMs(1000)
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"item": "inprogress",
		})
		sleepMs(100)
		c <- fs.Event{Name: "notinprogress", Type: fs.NonRemove}
		sleepMs(800)
	}

	expectedBatches := []expectedBatch{
		{[]string{"notinprogress"}, 2000, 3500},
	}

	testScenario(t, "InProgress", testCase, expectedBatches)
}

// TestDelay checks that recurring changes to the same path are delayed
// and different types separated and ordered correctly
func TestDelay(t *testing.T) {
	file := filepath.Join("parent", "file")
	delayed := "delayed"
	del := "deleted"
	both := filepath.Join("parent", "sub", "both")
	testCase := func(c chan<- fs.Event) {
		sleepMs(200)
		c <- fs.Event{Name: file, Type: fs.NonRemove}
		delay := time.Duration(300) * time.Millisecond
		timer := time.NewTimer(delay)
		<-timer.C
		timer.Reset(delay)
		c <- fs.Event{Name: delayed, Type: fs.NonRemove}
		c <- fs.Event{Name: both, Type: fs.NonRemove}
		c <- fs.Event{Name: both, Type: fs.Remove}
		c <- fs.Event{Name: del, Type: fs.Remove}
		for i := 0; i < 9; i++ {
			<-timer.C
			timer.Reset(delay)
			c <- fs.Event{Name: delayed, Type: fs.NonRemove}
		}
		<-timer.C
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		{[]string{file}, 500, 2500},
		{[]string{delayed}, 2500, 4500},
		{[]string{both}, 2500, 4500},
		{[]string{del}, 2500, 4500},
		{[]string{delayed}, 3600, 7000},
	}

	testScenario(t, "Delay", testCase, expectedBatches)
}

func getEventPaths(dir *eventDir, dirPath string, a *aggregator) []string {
	var paths []string
	for childName, childDir := range dir.dirs {
		for _, path := range getEventPaths(childDir, filepath.Join(dirPath, childName), a) {
			paths = append(paths, path)
		}
	}
	for name := range dir.events {
		paths = append(paths, filepath.Join(dirPath, name))
	}
	return paths
}

func sleepMs(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

func durationMs(ms int) time.Duration {
	return time.Duration(ms) * time.Millisecond
}

func compareBatchToExpected(t *testing.T, batch []string, expectedPaths []string) {
	for _, expected := range expectedPaths {
		expected = filepath.Clean(expected)
		found := false
		for i, received := range batch {
			if expected == received {
				found = true
				batch = append(batch[:i], batch[i+1:]...)
				break
			}
		}
		if !found {
			t.Errorf("Did not receive event %s", expected)
		}
	}
	for _, received := range batch {
		t.Errorf("Received unexpected event %s", received)
	}
}

func testScenario(t *testing.T, name string, testCase func(c chan<- fs.Event), expectedBatches []expectedBatch) {
	ctx, cancel := context.WithCancel(context.Background())
	eventChan := make(chan fs.Event)
	watchChan := make(chan []string)

	folderCfg := defaultFolderCfg.Copy()
	folderCfg.ID = name
	a := newAggregator(folderCfg, ctx)
	a.notifyTimeout = testNotifyTimeout

	startTime := time.Now()
	go a.mainLoop(eventChan, watchChan, defaultCfg)

	sleepMs(20)

	go testCase(eventChan)

	testAggregatorOutput(t, watchChan, expectedBatches, startTime)

	cancel()
}

func testAggregatorOutput(t *testing.T, fsWatchChan <-chan []string, expectedBatches []expectedBatch, startTime time.Time) {
	var received []string
	var elapsedTime time.Duration
	batchIndex := 0
	timeout := time.NewTimer(10 * time.Second)
	for {
		select {
		case <-timeout.C:
			t.Errorf("Timeout: Received only %d batches (%d expected)", batchIndex, len(expectedBatches))
			return
		case received = <-fsWatchChan:
		}

		elapsedTime = time.Since(startTime)
		expected := expectedBatches[batchIndex]

		if runtime.GOOS != "darwin" {
			switch {
			case elapsedTime < durationMs(expected.afterMs):
				t.Errorf("Received batch %d after %v (too soon)", batchIndex+1, elapsedTime)

			case elapsedTime > durationMs(expected.beforeMs):
				t.Errorf("Received batch %d after %v (too late)", batchIndex+1, elapsedTime)
			}
		}

		if len(received) != len(expected.paths) {
			t.Errorf("Received %v events instead of %v for batch %v", len(received), len(expected.paths), batchIndex+1)
		}
		compareBatchToExpected(t, received, expected.paths)

		batchIndex++
		if batchIndex == len(expectedBatches) {
			// received everything we expected to
			return
		}
	}
}

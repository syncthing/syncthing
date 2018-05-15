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

	ret := m.Run()

	maxFiles = 512
	maxFilesPerDir = 128

	os.Exit(ret)
}

const (
	testNotifyDelayS   = 1
	testNotifyTimeout  = 2 * time.Second
	timeoutWithinBatch = time.Second
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

// Represents possibly multiple (different event types) expected paths from
// aggregation, that should be received back to back.
type expectedBatch struct {
	paths    [][]string
	afterMs  int
	beforeMs int
}

// TestAggregate checks whether maxFilesPerDir+1 events in one dir are
// aggregated to parent dir
func TestAggregate(t *testing.T) {
	inProgress := make(map[string]struct{})

	folderCfg := defaultFolderCfg.Copy()
	folderCfg.ID = "Aggregate"
	ctx, _ := context.WithCancel(context.Background())
	a := newAggregator(folderCfg, ctx)

	// checks whether maxFilesPerDir events in one dir are kept as is
	for i := 0; i < maxFilesPerDir; i++ {
		a.newEvent(fs.Event{filepath.Join("parent", strconv.Itoa(i)), fs.NonRemove}, inProgress)
	}
	if l := len(getEventPaths(a.root, ".", a)); l != maxFilesPerDir {
		t.Errorf("Unexpected number of events stored, got %v, expected %v", l, maxFilesPerDir)
	}

	// checks whether maxFilesPerDir+1 events in one dir are aggregated to parent dir
	a.newEvent(fs.Event{filepath.Join("parent", "new"), fs.NonRemove}, inProgress)
	compareBatchToExpectedDirect(t, getEventPaths(a.root, ".", a), []string{"parent"})

	// checks that adding an event below "parent" does not change anything
	a.newEvent(fs.Event{filepath.Join("parent", "extra"), fs.NonRemove}, inProgress)
	compareBatchToExpectedDirect(t, getEventPaths(a.root, ".", a), []string{"parent"})

	// again test aggregation in "parent" but with event in subdirs
	a = newAggregator(folderCfg, ctx)
	for i := 0; i < maxFilesPerDir; i++ {
		a.newEvent(fs.Event{filepath.Join("parent", strconv.Itoa(i)), fs.NonRemove}, inProgress)
	}
	a.newEvent(fs.Event{filepath.Join("parent", "sub", "new"), fs.NonRemove}, inProgress)
	compareBatchToExpectedDirect(t, getEventPaths(a.root, ".", a), []string{"parent"})

	// test aggregation in root
	a = newAggregator(folderCfg, ctx)
	for i := 0; i < maxFiles; i++ {
		a.newEvent(fs.Event{strconv.Itoa(i), fs.NonRemove}, inProgress)
	}
	if len(getEventPaths(a.root, ".", a)) != maxFiles {
		t.Errorf("Unexpected number of events stored in root")
	}
	a.newEvent(fs.Event{filepath.Join("parent", "sub", "new"), fs.NonRemove}, inProgress)
	compareBatchToExpectedDirect(t, getEventPaths(a.root, ".", a), []string{"."})

	// checks that adding an event when "." is already stored is a noop
	a.newEvent(fs.Event{"anythingelse", fs.NonRemove}, inProgress)
	compareBatchToExpectedDirect(t, getEventPaths(a.root, ".", a), []string{"."})

	a = newAggregator(folderCfg, ctx)
	filesPerDir := maxFilesPerDir / 2
	dirs := make([]string, maxFiles/filesPerDir+1)
	for i := 0; i < maxFiles/filesPerDir+1; i++ {
		dirs[i] = "dir" + strconv.Itoa(i)
	}
	for _, dir := range dirs {
		for i := 0; i < filesPerDir; i++ {
			a.newEvent(fs.Event{filepath.Join(dir, strconv.Itoa(i)), fs.NonRemove}, inProgress)
		}
	}
	compareBatchToExpectedDirect(t, getEventPaths(a.root, ".", a), []string{"."})
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
		{[][]string{{"notinprogress"}}, 2000, 3500},
	}

	testScenario(t, "InProgress", testCase, expectedBatches)
}

// TestDelay checks that recurring changes to the same path are delayed
// and different types separated and ordered correctly
func TestDelay(t *testing.T) {
	file := filepath.Join("parent", "file")
	delayed := "delayed"
	del := "deleted"
	delAfter := "deletedAfter"
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
		c <- fs.Event{Name: delAfter, Type: fs.Remove}
		<-timer.C
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		{[][]string{{file}}, 500, 2500},
		{[][]string{{delayed}, {both}, {del}}, 2500, 4500},
		{[][]string{{delayed}, {delAfter}}, 3600, 7000},
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

func compareBatchToExpected(batch []string, expectedPaths []string) (missing []string, unexpected []string) {
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
			missing = append(missing, expected)
		}
	}
	for _, received := range batch {
		unexpected = append(unexpected, received)
	}
	return missing, unexpected
}

func compareBatchToExpectedDirect(t *testing.T, batch []string, expectedPaths []string) {
	t.Helper()
	missing, unexpected := compareBatchToExpected(batch, expectedPaths)
	for _, p := range missing {
		t.Errorf("Did not receive event %s", p)
	}
	for _, p := range unexpected {
		t.Errorf("Received unexpected event %s", p)
	}
}

func testScenario(t *testing.T, name string, testCase func(c chan<- fs.Event), expectedBatches []expectedBatch) {
	t.Helper()
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
	t.Helper()
	var received []string
	var elapsedTime time.Duration
	var batchIndex, innerIndex int
	timeout := time.NewTimer(10 * time.Second)
	for {
		select {
		case <-timeout.C:
			t.Errorf("Timeout: Received only %d batches (%d expected)", batchIndex, len(expectedBatches))
			return
		case received = <-fsWatchChan:
		}

		if batchIndex >= len(expectedBatches) {
			t.Errorf("Received batch %d, expected only %d", batchIndex+1, len(expectedBatches))
			continue
		}

		if runtime.GOOS != "darwin" {
			now := time.Since(startTime)
			if innerIndex == 0 {
				switch {
				case now < durationMs(expectedBatches[batchIndex].afterMs):
					t.Errorf("Received batch %d after %v (too soon)", batchIndex+1, elapsedTime)

				case now > durationMs(expectedBatches[batchIndex].beforeMs):
					t.Errorf("Received batch %d after %v (too late)", batchIndex+1, elapsedTime)
				}
			} else if innerTime := now - elapsedTime; innerTime > timeoutWithinBatch {
				t.Errorf("Receive part %d of batch %d after %v (too late)", innerIndex+1, batchIndex+1, innerTime)
			}
			elapsedTime = now
		}

		expected := expectedBatches[batchIndex].paths[innerIndex]

		if len(received) != len(expected) {
			t.Errorf("Received %v events instead of %v for batch %v", len(received), len(expected), batchIndex+1)
		}
		missing, unexpected := compareBatchToExpected(received, expected)
		for _, p := range missing {
			t.Errorf("Did not receive event %s in batch %d (%d)", p, batchIndex+1, innerIndex+1)
		}
		for _, p := range unexpected {
			t.Errorf("Received unexpected event %s in batch %d (%d)", p, batchIndex+1, innerIndex+1)
		}

		if innerIndex == len(expectedBatches[batchIndex].paths)-1 {
			if batchIndex == len(expectedBatches)-1 {
				// received everything we expected to
				return
			}
			innerIndex = 0
			batchIndex++
		} else {
			innerIndex++
		}
	}
}

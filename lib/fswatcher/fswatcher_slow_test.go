// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build !solaris,!darwin solaris,cgo darwin,cgo

package fswatcher

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
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
	testFs = fs.NewFilesystem(fs.FilesystemTypeBasic, testDirAbs)

	maxFiles = 32
	maxFilesPerDir = 8
	defer func() {
		maxFiles = 512
		maxFilesPerDir = 128
	}()
	os.Exit(m.Run())
}

const (
	testDir           = "temporary_test_fswatcher"
	testNotifyDelayS  = 1
	testNotifyTimeout = time.Duration(2) * time.Second
)

var (
	testDirAbs string
	testFs     fs.Filesystem
)

type expectedBatch struct {
	paths    []string
	afterMs  int
	beforeMs int
}

// TestTemplate illustrates how a test can be created.
// It also checks some basic operations like file creation, deletion and folder
// creation, and behaviour like reactivating timer on new event
func TestTemplate(t *testing.T) {
	// create dirs/files that should exist before FS watching
	oldfile := createTestFile(t, "oldfile")

	// dir/file manipulations during FS watching
	file1 := "file1"
	file2 := "dir1/file2"
	dir1 := "dir1"
	testCase := func(watcher Service) {
		// test timer reactivation
		sleepMs(1100)
		createTestFile(t, file1)
		createTestDir(t, dir1)
		sleepMs(1100)
		createTestFile(t, file2)
		deleteTestFile(t, oldfile)
		sleepMs(1000)
		deleteTestFile(t, file1)
		deleteTestDir(t, dir1)
	}

	// batches that we expect to receive with time interval in milliseconds
	expectedBatches := []expectedBatch{
		{[]string{file1, dir1}, 2000, 3000},
		{[]string{file2}, 3000, 4000},
		{[]string{oldfile}, 4200, 5500},
		{[]string{file1, dir1}, 5200, 7000},
	}

	testScenario(t, "Template", testCase, expectedBatches)
}

func TestRename(t *testing.T) {
	oldfile := createTestFile(t, "oldfile")
	newfile := "newfile"

	testCase := func(watcher Service) {
		sleepMs(400)
		renameTestFile(t, oldfile, newfile)
	}

	var expectedBatches []expectedBatch
	// Only on these platforms the removed file can be differentiated from
	// the created file during renaming
	if runtime.GOOS == "windows" || runtime.GOOS == "linux" || runtime.GOOS == "solaris" {
		expectedBatches = []expectedBatch{
			{[]string{newfile}, 900, 1900},
			{[]string{oldfile}, 2900, 4000},
		}
	} else {
		expectedBatches = []expectedBatch{
			{[]string{newfile, oldfile}, 2900, 4000},
		}
	}

	testScenario(t, "Rename", testCase, expectedBatches)
}

// TestAggregate checks whether maxFilesPerDir+1 events in one dir are
// aggregated to parent dir
func TestAggregate(t *testing.T) {
	parent := createTestDir(t, "parent")
	files := make([]string, maxFilesPerDir+1)
	for i := 0; i < maxFilesPerDir+1; i++ {
		files[i] = filepath.Join(parent, strconv.Itoa(i))
	}
	testCase := func(watcher Service) {
		for _, file := range files {
			createTestFile(t, file)
		}
	}

	expectedBatches := []expectedBatch{
		{[]string{parent}, 900, 1600},
	}

	testScenario(t, "Aggregate", testCase, expectedBatches)
}

// TestAggregateParent checks whether maxFilesPerDir events in one dir and
// event in a subdir of dir are aggregated
func TestAggregateParent(t *testing.T) {
	parent := createTestDir(t, "parent")
	createTestDir(t, filepath.Join(parent, "dir"))
	files := make([]string, maxFilesPerDir)
	for i := 0; i < maxFilesPerDir; i++ {
		files[i] = filepath.Join(parent, strconv.Itoa(i))
	}
	childFile := "parent/dir/childFile"
	testCase := func(watcher Service) {
		for _, file := range files {
			createTestFile(t, file)
		}
		sleepMs(50)
		createTestFile(t, childFile)
	}

	expectedBatches := []expectedBatch{
		{[]string{parent}, 900, 1600},
	}

	testScenario(t, "AggregateParent", testCase, expectedBatches)
}

// TestRootAggreagate checks that maxFiles+1 events in root dir are aggregated
func TestRootAggregate(t *testing.T) {
	files := make([]string, maxFiles+1)
	for i := 0; i < maxFiles+1; i++ {
		files[i] = strconv.Itoa(i)
	}
	testCase := func(watcher Service) {
		for _, file := range files {
			createTestFile(t, file)
		}
	}

	expectedBatches := []expectedBatch{
		{[]string{"."}, 900, 1600},
	}

	testScenario(t, "RootAggregate", testCase, expectedBatches)
}

// TestRootNotAggreagate checks that maxFilesPerDir+1 events in root dir are
// not aggregated
func TestRootNotAggregate(t *testing.T) {
	files := make([]string, maxFilesPerDir+1)
	for i := 0; i < maxFilesPerDir+1; i++ {
		files[i] = strconv.Itoa(i)
	}
	testCase := func(watcher Service) {
		for _, file := range files {
			createTestFile(t, file)
		}
	}

	expectedBatches := []expectedBatch{
		{files[:], 900, 1600},
	}

	testScenario(t, "RootNotAggregate", testCase, expectedBatches)
}

// TestOverflow checks that the entire folder is scanned when maxFiles is reached
func TestOverflow(t *testing.T) {
	filesPerDir := maxFiles / 5
	dirs := make([]string, maxFiles/filesPerDir+1)
	for i := 0; i < maxFiles/filesPerDir+1; i++ {
		dirs[i] = createTestDir(t, "dir"+strconv.Itoa(i))
	}
	testCase := func(watcher Service) {
		for _, dir := range dirs {
			for i := 0; i < filesPerDir; i++ {
				createTestFile(t, filepath.Join(dir, "file"+strconv.Itoa(i)))
			}
		}
	}

	expectedBatches := []expectedBatch{
		{[]string{"."}, 900, 1600},
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
		panic(fmt.Sprintf("Failed to create directory %s: %s", outDir, err))
	}
	createTestFile(t, "dir/file")
	testCase := func(watcher Service) {
		sleepMs(400)
		if err := os.Rename(filepath.Join(testDir, dir), filepath.Join(outDir, dir)); err != nil {
			panic(err)
		}
		if err := os.RemoveAll(outDir); err != nil {
			panic(err)
		}
	}

	expectedBatches := []expectedBatch{
		{[]string{dir}, 2400, 4000},
	}

	testScenario(t, "Outside", testCase, expectedBatches)
}

// TestUpdateIgnores checks that updating ignores has the desired effect
func TestUpdateIgnores(t *testing.T) {
	stignore := `
	a*
	`
	pats := ignore.New(testFs)
	err := pats.Parse(bytes.NewBufferString(stignore), ".stignore")
	if err != nil {
		t.Fatal(err)
	}

	testCase := func(watcher Service) {
		createTestFile(t, "afirst")
		sleepMs(1100)
		watcher.UpdateIgnores(pats)
		sleepMs(100)
		createTestFile(t, "asecond")
		sleepMs(1600)
	}

	expectedBatches := []expectedBatch{
		{[]string{"afirst"}, 900, 1600},
	}

	testScenario(t, "UpdateIgnores", testCase, expectedBatches)
}

// TestInProgress checks that ignoring files currently edited by Syncthing works
func TestInProgress(t *testing.T) {
	testCase := func(service Service) {
		events.Default.Log(events.ItemStarted, map[string]string{
			"item": "inprogress",
		})
		sleepMs(100)
		createTestFile(t, "inprogress")
		sleepMs(1000)
		events.Default.Log(events.ItemFinished, map[string]interface{}{
			"item": "inprogress",
		})
		sleepMs(100)
		createTestFile(t, "notinprogress")
		sleepMs(800)
	}

	expectedBatches := []expectedBatch{
		{[]string{"notinprogress"}, 2000, 3500},
	}

	testScenario(t, "TestInProgress", testCase, expectedBatches)
}

func testFsWatcher(t *testing.T, name string) Service {
	folderCfg := config.FolderConfiguration{
		ID:              name,
		FilesystemType:  fs.FilesystemTypeBasic,
		Path:            testDirAbs,
		FSWatcherDelayS: testNotifyDelayS,
	}
	cfg := config.Configuration{
		Folders: []config.FolderConfiguration{folderCfg},
	}
	wrapper := config.Wrap("", cfg)
	testWatcher := New(folderCfg, wrapper, nil)
	testWatcher.(*watcher).notifyTimeout = testNotifyTimeout
	return testWatcher
}

// path relative to folder root
func renameTestFile(t *testing.T, old string, new string) {
	if err := testFs.Rename(old, new); err != nil {
		panic(fmt.Sprintf("Failed to rename %s to %s: %s", old, new, err))
	}
}

// path relative to folder root
func deleteTestFile(t *testing.T, file string) {
	if err := testFs.Remove(file); err != nil {
		panic(fmt.Sprintf("Failed to delete %s: %s", file, err))
	}
}

// path relative to folder root
func deleteTestDir(t *testing.T, dir string) {
	if err := testFs.RemoveAll(dir); err != nil {
		panic(fmt.Sprintf("Failed to delete %s: %s", dir, err))
	}
}

// path relative to folder root, also creates parent dirs if necessary
func createTestFile(t *testing.T, file string) string {
	if err := testFs.MkdirAll(filepath.Dir(file), 0755); err != nil {
		panic(fmt.Sprintf("Failed to parent directory for %s: %s", file, err))
	}
	handle, err := testFs.Create(file)
	if err != nil {
		panic(fmt.Sprintf("Failed to create test file %s: %s", file, err))
	}
	handle.Close()
	return file
}

// path relative to folder root
func createTestDir(t *testing.T, dir string) string {
	if err := testFs.MkdirAll(dir, 0755); err != nil {
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

func compareBatchToExpected(t *testing.T, batch []string, expectedPaths []string, batchIndex int) {
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
			t.Errorf("Did not receive event %s in batch %d", expected, batchIndex+1)
		}
	}
	for _, received := range batch {
		t.Errorf("Received unexpected event %s in batch %d", received, batchIndex+1)
	}
}

func testScenario(t *testing.T, name string, testCase func(watcher Service), expectedBatches []expectedBatch) {
	createTestDir(t, ".")

	// Tests pick up the previously created files/dirs, probably because
	// they get flushed to disked with a delay.
	initDelayMs := 500
	if runtime.GOOS == "darwin" {
		initDelayMs = 900
	}
	sleepMs(initDelayMs)

	fsWatcher := testFsWatcher(t, name)

	abort := make(chan struct{})

	startTime := time.Now()

	go fsWatcher.Serve()

	// To allow using round numbers in expected times
	sleepMs(10)
	go testFsWatcherOutput(t, fsWatcher.C(), expectedBatches, startTime, abort)

	timeout := time.NewTimer(time.Duration(expectedBatches[len(expectedBatches)-1].beforeMs+100) * time.Millisecond)
	testCase(fsWatcher)
	<-timeout.C

	abort <- struct{}{}
	fsWatcher.Stop()
	os.RemoveAll(testDir)
	<-abort
}

func testFsWatcherOutput(t *testing.T, fsWatchChan <-chan []string, expectedBatches []expectedBatch, startTime time.Time, abort chan struct{}) {
	var received []string
	var elapsedTime time.Duration
	batchIndex := 0
	for {
		select {
		case <-abort:
			if batchIndex != len(expectedBatches) {
				t.Errorf("Received only %d batches (%d expected)", batchIndex, len(expectedBatches))
			}
			abort <- struct{}{}
			return
		case received = <-fsWatchChan:
		}

		if batchIndex >= len(expectedBatches) {
			t.Errorf("Received batch %d (only %d expected)", batchIndex+1, len(expectedBatches))
			continue
		}

		elapsedTime = time.Since(startTime)
		expected := expectedBatches[batchIndex]
		switch {
		case elapsedTime < durationMs(expected.afterMs):
			t.Errorf("Received batch %d after %v (too soon)", batchIndex+1, elapsedTime)

		case elapsedTime > durationMs(expected.beforeMs):
			t.Errorf("Received batch %d after %v (too late)", batchIndex+1, elapsedTime)

		case len(received) != len(expected.paths):
			t.Errorf("Received %v events instead of %v for batch %v", len(received), len(expected.paths), batchIndex+1)
		}
		compareBatchToExpected(t, received, expected.paths, batchIndex)
		batchIndex++
	}
}

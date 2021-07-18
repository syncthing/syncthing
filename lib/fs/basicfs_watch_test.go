// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build !solaris,!darwin solaris,cgo darwin,cgo
// +build !ios

package fs

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/syncthing/notify"
)

func TestMain(m *testing.M) {
	if err := os.RemoveAll(testDir); err != nil {
		panic(err)
	}

	dir, err := filepath.Abs(".")
	if err != nil {
		panic("Cannot get absolute path to working dir")
	}

	dir, err = evalSymlinks(dir)
	if err != nil {
		panic("Cannot get real path to working dir")
	}

	testDirAbs = filepath.Join(dir, testDir)
	if runtime.GOOS == "windows" {
		testDirAbs = longFilenameSupport(testDirAbs)
	}

	testFs = NewFilesystem(FilesystemTypeBasic, testDirAbs)

	backendBuffer = 10

	exitCode := m.Run()

	backendBuffer = 500
	os.RemoveAll(testDir)

	os.Exit(exitCode)
}

const (
	testDir        = "testdata"
	failsOnOpenBSD = "Fails on OpenBSD. See https://github.com/rjeczalik/notify/issues/172"
)

var (
	testDirAbs string
	testFs     Filesystem
)

func TestWatchIgnore(t *testing.T) {
	if runtime.GOOS == "openbsd" {
		t.Skip(failsOnOpenBSD)
	}
	name := "ignore"

	file := "file"
	ignored := "ignored"

	testCase := func() {
		createTestFile(name, file)
		createTestFile(name, ignored)
	}

	expectedEvents := []Event{
		{file, NonRemove},
	}
	allowedEvents := []Event{
		{name, NonRemove},
	}

	testScenario(t, name, testCase, expectedEvents, allowedEvents, fakeMatcher{ignore: filepath.Join(name, ignored), skipIgnoredDirs: true}, false)
}

func TestWatchInclude(t *testing.T) {
	if runtime.GOOS == "openbsd" {
		t.Skip(failsOnOpenBSD)
	}
	name := "include"

	file := "file"
	ignored := "ignored"
	testFs.MkdirAll(filepath.Join(name, ignored), 0777)
	included := filepath.Join(ignored, "included")

	testCase := func() {
		createTestFile(name, file)
		createTestFile(name, included)
	}

	expectedEvents := []Event{
		{file, NonRemove},
		{included, NonRemove},
	}
	allowedEvents := []Event{
		{name, NonRemove},
	}

	testScenario(t, name, testCase, expectedEvents, allowedEvents, fakeMatcher{ignore: filepath.Join(name, ignored), include: filepath.Join(name, included)}, false)
}

func TestWatchRename(t *testing.T) {
	if runtime.GOOS == "openbsd" {
		t.Skip(failsOnOpenBSD)
	}
	name := "rename"

	old := createTestFile(name, "oldfile")
	new := "newfile"

	testCase := func() {
		renameTestFile(name, old, new)
	}

	destEvent := Event{new, Remove}
	// Only on these platforms the removed file can be differentiated from
	// the created file during renaming
	if runtime.GOOS == "windows" || runtime.GOOS == "linux" || runtime.GOOS == "solaris" || runtime.GOOS == "freebsd" {
		destEvent = Event{new, NonRemove}
	}
	expectedEvents := []Event{
		{old, Remove},
		destEvent,
	}
	allowedEvents := []Event{
		{name, NonRemove},
	}

	// set the "allow others" flag because we might get the create of
	// "oldfile" initially
	testScenario(t, name, testCase, expectedEvents, allowedEvents, fakeMatcher{}, false)
}

// TestWatchWinRoot checks that a watch at a drive letter does not panic due to
// out of root event on every event.
// https://github.com/syncthing/syncthing/issues/5695
func TestWatchWinRoot(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows specific test")
	}

	outChan := make(chan Event)
	backendChan := make(chan notify.EventInfo, backendBuffer)
	errChan := make(chan error)

	ctx, cancel := context.WithCancel(context.Background())

	// testFs is Filesystem, but we need BasicFilesystem here
	root := `D:\`
	fs := newBasicFilesystem(root)
	watch, roots, err := fs.watchPaths(".")
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	defer func() {
		cancel()
		<-done
	}()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error(r)
			}
			cancel()
		}()
		fs.watchLoop(ctx, ".", roots, backendChan, outChan, errChan, fakeMatcher{})
		close(done)
	}()

	// filepath.Dir as watch has a /... suffix
	name := "foo"
	backendChan <- fakeEventInfo(filepath.Join(filepath.Dir(watch), name))

	select {
	case <-time.After(10 * time.Second):
		cancel()
		t.Errorf("Timed out before receiving event")
	case ev := <-outChan:
		if ev.Name != name {
			t.Errorf("Unexpected event %v, expected %v", ev.Name, name)
		}
	case err := <-errChan:
		t.Error("Received fatal watch error:", err)
	case <-ctx.Done():
	}
}

// TestWatchOutside checks that no changes from outside the folder make it in
func TestWatchOutside(t *testing.T) {
	expectErrorForPath(t, filepath.Join(filepath.Dir(testDirAbs), "outside"))

	rootWithoutSlash := strings.TrimRight(filepath.ToSlash(testDirAbs), "/")
	expectErrorForPath(t, rootWithoutSlash+"outside")
	expectErrorForPath(t, rootWithoutSlash+"outside/thing")
}

func expectErrorForPath(t *testing.T, path string) {
	outChan := make(chan Event)
	backendChan := make(chan notify.EventInfo, backendBuffer)
	errChan := make(chan error)

	ctx, cancel := context.WithCancel(context.Background())

	// testFs is Filesystem, but we need BasicFilesystem here
	fs := newBasicFilesystem(testDirAbs)

	done := make(chan struct{})
	go func() {
		fs.watchLoop(ctx, ".", []string{testDirAbs}, backendChan, outChan, errChan, fakeMatcher{})
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	backendChan <- fakeEventInfo(path)

	select {
	case <-time.After(10 * time.Second):
		t.Errorf("Timed out before receiving error")
	case e := <-outChan:
		t.Errorf("Unexpected passed through event %v", e)
	case <-errChan:
	case <-ctx.Done():
	}
}

func TestWatchSubpath(t *testing.T) {
	outChan := make(chan Event)
	backendChan := make(chan notify.EventInfo, backendBuffer)
	errChan := make(chan error)

	ctx, cancel := context.WithCancel(context.Background())

	// testFs is Filesystem, but we need BasicFilesystem here
	fs := newBasicFilesystem(testDirAbs)

	abs, _ := fs.rooted("sub")
	done := make(chan struct{})
	go func() {
		fs.watchLoop(ctx, "sub", []string{testDirAbs}, backendChan, outChan, errChan, fakeMatcher{})
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	backendChan <- fakeEventInfo(filepath.Join(abs, "file"))

	timeout := time.NewTimer(2 * time.Second)
	select {
	case <-timeout.C:
		t.Errorf("Timed out before receiving an event")
		cancel()
	case ev := <-outChan:
		if ev.Name != filepath.Join("sub", "file") {
			t.Errorf("While watching a subfolder, received an event with unexpected path %v", ev.Name)
		}
	case err := <-errChan:
		t.Error("Received fatal watch error:", err)
	}

	cancel()
}

// TestWatchOverflow checks that an event at the root is sent when maxFiles is reached
func TestWatchOverflow(t *testing.T) {
	if runtime.GOOS == "openbsd" {
		t.Skip(failsOnOpenBSD)
	}
	name := "overflow"

	expectedEvents := []Event{
		{".", NonRemove},
	}

	allowedEvents := []Event{
		{name, NonRemove},
	}

	testCase := func() {
		for i := 0; i < 5*backendBuffer; i++ {
			file := "file" + strconv.Itoa(i)
			createTestFile(name, file)
			allowedEvents = append(allowedEvents, Event{file, NonRemove})
		}
	}

	testScenario(t, name, testCase, expectedEvents, allowedEvents, fakeMatcher{}, false)
}

func TestWatchErrorLinuxInterpretation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("testing of linux specific error codes")
	}

	var errTooManyFiles = &os.PathError{
		Op:   "error while traversing",
		Path: "foo",
		Err:  syscall.Errno(24),
	}
	var errNoSpace = &os.PathError{
		Op:   "error while traversing",
		Path: "bar",
		Err:  syscall.Errno(28),
	}

	if !reachedMaxUserWatches(errTooManyFiles) {
		t.Error("Underlying error syscall.Errno(24) should be recognised to be about inotify limits.")
	}
	if !reachedMaxUserWatches(errNoSpace) {
		t.Error("Underlying error syscall.Errno(28) should be recognised to be about inotify limits.")
	}
	err := errors.New("Another error")
	if reachedMaxUserWatches(err) {
		t.Errorf("This error does not concern inotify limits: %#v", err)
	}
}

func TestWatchSymlinkedRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Involves symlinks")
	}

	name := "symlinkedRoot"
	if err := testFs.MkdirAll(name, 0755); err != nil {
		panic(fmt.Sprintf("Failed to create directory %s: %s", name, err))
	}
	defer testFs.RemoveAll(name)

	root := filepath.Join(name, "root")
	if err := testFs.MkdirAll(root, 0777); err != nil {
		panic(err)
	}
	link := filepath.Join(name, "link")

	if err := testFs.CreateSymlink(filepath.Join(testFs.URI(), root), link); err != nil {
		panic(err)
	}

	linkedFs := NewFilesystem(FilesystemTypeBasic, filepath.Join(testFs.URI(), link))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, _, err := linkedFs.Watch(".", fakeMatcher{}, ctx, false); err != nil {
		panic(err)
	}

	if err := linkedFs.MkdirAll("foo", 0777); err != nil {
		panic(err)
	}

	// Give the panic some time to happen
	sleepMs(100)
}

func TestUnrootedChecked(t *testing.T) {
	fs := newBasicFilesystem(testDirAbs)
	if unrooted, err := fs.unrootedChecked("/random/other/path", []string{testDirAbs}); err == nil {
		t.Error("unrootedChecked did not return an error on outside path, but returned", unrooted)
	}
}

func TestWatchIssue4877(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows specific test")
	}

	name := "Issue4877"

	file := "file"

	testCase := func() {
		createTestFile(name, file)
	}

	expectedEvents := []Event{
		{file, NonRemove},
	}
	allowedEvents := []Event{
		{name, NonRemove},
	}

	volName := filepath.VolumeName(testDirAbs)
	if volName == "" {
		t.Fatalf("Failed to get volume name for path %v", testDirAbs)
	}
	origTestFs := testFs
	testFs = NewFilesystem(FilesystemTypeBasic, strings.ToLower(volName)+strings.ToUpper(testDirAbs[len(volName):]))
	defer func() {
		testFs = origTestFs
	}()

	testScenario(t, name, testCase, expectedEvents, allowedEvents, fakeMatcher{}, false)
}

func TestWatchModTime(t *testing.T) {
	name := "modtime"

	file := createTestFile(name, "foo")
	path := filepath.Join(name, file)
	now := time.Now()
	before := now.Add(-10 * time.Second)
	if err := testFs.Chtimes(path, before, before); err != nil {
		t.Fatal(err)
	}

	testCase := func() {
		if err := testFs.Chtimes(path, now, now); err != nil {
			t.Error(err)
		}
	}

	expectedEvents := []Event{
		{file, NonRemove},
	}

	var allowedEvents []Event
	// Apparently an event for the parent is also sent on mac
	if runtime.GOOS == "darwin" {
		allowedEvents = []Event{
			{name, NonRemove},
		}
	}

	testScenario(t, name, testCase, expectedEvents, allowedEvents, fakeMatcher{}, false)
}

func TestModifyFile(t *testing.T) {
	name := "modify"

	old := createTestFile(name, "file")
	modifyTestFile(name, old, "syncthing")

	testCase := func() {
		modifyTestFile(name, old, "modified")
	}

	expectedEvents := []Event{
		{old, NonRemove},
	}
	allowedEvents := []Event{
		{name, NonRemove},
	}

	sleepMs(1000)
	testScenario(t, name, testCase, expectedEvents, allowedEvents, fakeMatcher{}, false)
}

func TestTruncateFileOnly(t *testing.T) {
	name := "truncate"

	file := createTestFile(name, "file")
	modifyTestFile(name, file, "syncthing")

	// modified the content to empty use os.WriteFile will first truncate the file
	// (/os/file.go:696) then write nothing. This logic is also used in many editors,
	// such as when emptying a file in VSCode or JetBrain
	//
	// darwin will only modified the inode's metadata, such us mtime, file size, etc.
	// but would not modified the file directly, so FSEvent 'FSEventsModified' will not
	// be received
	//
	// we should watch the FSEvent 'FSEventsInodeMetaMod' to watch the Inode metadata,
	// and that should be considered as an NonRemove Event
	//
	// notify also considered FSEventsInodeMetaMod as Write Event
	// /watcher_fsevents.go:89
	testCase := func() {
		modifyTestFile(name, file, "")
	}

	expectedEvents := []Event{
		{file, NonRemove},
	}
	allowedEvents := []Event{
		{file, NonRemove},
	}

	sleepMs(1000)
	testScenario(t, name, testCase, expectedEvents, allowedEvents, fakeMatcher{}, true)
}

// path relative to folder root, also creates parent dirs if necessary
func createTestFile(name string, file string) string {
	joined := filepath.Join(name, file)
	if err := testFs.MkdirAll(filepath.Dir(joined), 0755); err != nil {
		panic(fmt.Sprintf("Failed to create parent directory for %s: %s", joined, err))
	}
	handle, err := testFs.Create(joined)
	if err != nil {
		panic(fmt.Sprintf("Failed to create test file %s: %s", joined, err))
	}
	handle.Close()
	return file
}

func renameTestFile(name string, old string, new string) {
	old = filepath.Join(name, old)
	new = filepath.Join(name, new)
	if err := testFs.Rename(old, new); err != nil {
		panic(fmt.Sprintf("Failed to rename %s to %s: %s", old, new, err))
	}
}

func modifyTestFile(name string, file string, content string) {
	joined := filepath.Join(testDirAbs, name, file)

	err := ioutil.WriteFile(joined, []byte(content), 0755)
	if err != nil {
		panic(fmt.Sprintf("Failed to modify test file %s: %s", joined, err))
	}
}

func sleepMs(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

func testScenario(t *testing.T, name string, testCase func(), expectedEvents, allowedEvents []Event, fm fakeMatcher, ignorePerms bool) {
	if err := testFs.MkdirAll(name, 0755); err != nil {
		panic(fmt.Sprintf("Failed to create directory %s: %s", name, err))
	}
	defer testFs.RemoveAll(name)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventChan, errChan, err := testFs.Watch(name, fm, ctx, ignorePerms)
	if err != nil {
		panic(err)
	}

	go testWatchOutput(t, name, eventChan, expectedEvents, allowedEvents, ctx, cancel)

	testCase()

	select {
	case <-time.After(10 * time.Second):
		t.Error("Timed out before receiving all expected events")
	case err := <-errChan:
		t.Error("Received fatal watch error:", err)
	case <-ctx.Done():
	}
}

func testWatchOutput(t *testing.T, name string, in <-chan Event, expectedEvents, allowedEvents []Event, ctx context.Context, cancel context.CancelFunc) {
	var expected = make(map[Event]struct{})
	for _, ev := range expectedEvents {
		ev.Name = filepath.Join(name, ev.Name)
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
			if len(allowedEvents) > 0 {
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

// Matches are done via direct comparison against both ignore and include
type fakeMatcher struct {
	ignore          string
	include         string
	skipIgnoredDirs bool
}

func (fm fakeMatcher) ShouldIgnore(name string) bool {
	return name != fm.include && name == fm.ignore
}

func (fm fakeMatcher) SkipIgnoredDirs() bool {
	return fm.skipIgnoredDirs
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

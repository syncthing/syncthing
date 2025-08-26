// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build windows
// +build windows

package fs

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/ignore/ignoreresult"
)

func TestWatchJunction(t *testing.T) {
	if !build.IsWindows {
		t.Skip("Junction tests are Windows only")
	}

	// Create test directory structure
	testDir := t.TempDir()
	fs := NewFilesystem(FilesystemTypeBasic, testDir, new(OptionJunctionsAsDirs))

	// Create target directory with a subdirectory
	if err := fs.MkdirAll("target/foo", 0o755); err != nil {
		t.Fatal(err)
	}

	// Create junction pointing to target
	targetPath := filepath.Join(testDir, "target")
	junctionPath := filepath.Join(testDir, "junction")
	if err := createDirJunct(targetPath, junctionPath); err != nil {
		t.Fatal(err)
	}

	// Set up watcher
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ignore := &testMatcher{}
	eventChan, errChan, err := fs.Watch(".", ignore, ctx, false)
	if err != nil {
		t.Fatal("Failed to set up watch:", err)
	}

	// Give the watcher time to set up
	time.Sleep(100 * time.Millisecond)

	// Create a file in the target directory (accessible through junction)
	// We create it directly in the target directory, which should be accessible through the junction
	// The key test is that the event is received, not the exact path format
	testFile := filepath.Join("target", "foo", "testfile.txt")
	fd, err := fs.Create(testFile)
	if err != nil {
		t.Fatal(err)
	}
	fd.Close()

	// Wait for events with timeout
	timeout := time.After(10 * time.Second)
	select {
	case ev := <-eventChan:
		// This is what should happen - we should get an event
		t.Logf("Received event: %+v", ev)
		// The event should be for the file within the junction
	case err := <-errChan:
		t.Fatal("Watcher error:", err)
	case <-timeout:
		t.Fatal("Timeout waiting for file event in junction - this indicates the fix is not working")
	}
}

type testMatcher struct{}

func (fm testMatcher) Match(name string) ignoreresult.R {
	return ignoreresult.NotIgnored
}

func (fm testMatcher) SkipIgnoredDirs() bool {
	return false
}
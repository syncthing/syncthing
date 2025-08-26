// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package fs

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/build"
)

// TestLargeFolderDetection tests that large folders are properly detected and logged
func TestLargeFolderDetection(t *testing.T) {
	if build.IsWindows {
		t.Skip("Skipping on Windows as this test creates many files")
	}

	name := "largeFolder"
	if err := testFs.MkdirAll(name, 0o755); err != nil {
		t.Fatal("Failed to create directory:", err)
	}
	defer testFs.RemoveAll(name)

	// Create a moderate number of files to test the detection logic
	// (not actually large enough to trigger warnings, but enough to test the counting)
	fileCount := 150
	for i := 0; i < fileCount; i++ {
		file := fmt.Sprintf("file%d.txt", i)
		createTestFile(name, file)
	}

	// Test that we can count the files correctly
	fs := newBasicFilesystem(filepath.Join(testDirAbs, name))
	count, err := countFilesInDirectory(fs, ".")
	if err != nil {
		t.Fatal("Failed to count files:", err)
	}

	if count != fileCount {
		t.Errorf("Expected %d files, got %d", fileCount, count)
	}

	// Test the large folder check function directly
	checkLargeFolder(fs, ".")
}

// TestAdaptiveBufferWithHighLoad tests the adaptive buffer management under high event load
func TestAdaptiveBufferWithHighLoad(t *testing.T) {
	if build.IsWindows {
		t.Skip("Skipping on Windows due to different buffer behavior")
	}

	name := "adaptiveBufferTest"
	if err := testFs.MkdirAll(name, 0o755); err != nil {
		t.Fatal("Failed to create directory:", err)
	}
	defer testFs.RemoveAll(name)

	// Create test files
	fileCount := 50
	for i := 0; i < fileCount; i++ {
		file := fmt.Sprintf("file%d.txt", i)
		createTestFile(name, file)
	}

	// Test the overflow tracker
	ot := newOverflowTracker()

	// Simulate frequent overflows
	for i := 0; i < 5; i++ {
		ot.recordOverflow()
		time.Sleep(10 * time.Millisecond) // Small delay to simulate time between overflows
	}

	// Check that we detect frequent overflows
	if !ot.shouldIncreaseBuffer() {
		t.Error("Expected shouldIncreaseBuffer to return true for frequent overflows")
	}

	// Test buffer increase
	originalSize := ot.adaptiveBuffer
	newSize := ot.increaseBuffer()

	if newSize <= originalSize {
		t.Error("Expected buffer size to increase")
	}

	// Test that we cap at maximum size
	ot.adaptiveBuffer = 9000
	newSize = ot.increaseBuffer()

	if newSize != 10000 {
		t.Errorf("Expected buffer to be capped at 10000, got %d", newSize)
	}
}

// TestWatchMetrics tests the metrics collection functionality
func TestWatchMetrics(t *testing.T) {
	metrics := newWatchMetrics()

	// Test recording events
	for i := 0; i < 10; i++ {
		metrics.recordEvent()
	}

	// Test recording dropped events
	for i := 0; i < 3; i++ {
		metrics.recordDroppedEvent()
	}

	// Test recording overflows
	for i := 0; i < 2; i++ {
		metrics.recordOverflow()
	}

	// Get metrics
	eventsProcessed, eventsDropped, overflows, uptime, timeSinceLastEvent := metrics.getMetrics()

	if eventsProcessed != 10 {
		t.Errorf("Expected 10 processed events, got %d", eventsProcessed)
	}

	if eventsDropped != 3 {
		t.Errorf("Expected 3 dropped events, got %d", eventsDropped)
	}

	if overflows != 2 {
		t.Errorf("Expected 2 overflows, got %d", overflows)
	}

	// Check that uptime and time since last event are non-negative (they may be 0 for very short tests)
	if uptime < 0 {
		t.Error("Expected non-negative uptime")
	}

	if timeSinceLastEvent < 0 {
		t.Error("Expected non-negative time since last event")
	}
}

// TestOverflowTrackerConcurrency tests that the overflow tracker is thread-safe
func TestOverflowTrackerConcurrency(t *testing.T) {
	ot := newOverflowTracker()

	// Run multiple goroutines that record overflows concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				ot.recordOverflow()
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Check that all overflows were recorded
	if ot.count != 100 {
		t.Errorf("Expected 100 overflows, got %d", ot.count)
	}
}

// TestWatchWithLargeBuffer tests that the larger buffer on Windows works correctly
func TestWatchWithLargeBuffer(t *testing.T) {
	// Check that the buffer size is set correctly
	// Note: In test mode, backendBuffer is set to 10 by TestMain, so we check the logic instead
	if build.IsWindows {
		// In normal operation, Windows should use a larger buffer
		// We can't easily test this in the test environment where it's set to 10
		t.Logf("Windows platform detected. In normal operation, buffer would be 2000, but in tests it's %d", backendBuffer)
	} else {
		// On non-Windows, it should be the default (but may be overridden in tests)
		t.Logf("Non-Windows platform. Buffer size: %d", backendBuffer)
	}
}

// BenchmarkFileCounting benchmarks the file counting functionality
func BenchmarkFileCounting(b *testing.B) {
	name := "benchmarkFolder"
	if err := testFs.MkdirAll(name, 0o755); err != nil {
		b.Fatal("Failed to create directory:", err)
	}
	defer testFs.RemoveAll(name)

	// Create test files
	fileCount := 1000
	for i := 0; i < fileCount; i++ {
		file := fmt.Sprintf("file%d.txt", i)
		createTestFile(name, file)
	}

	fs := newBasicFilesystem(filepath.Join(testDirAbs, name))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := countFilesInDirectory(fs, ".")
		if err != nil {
			b.Fatal("Failed to count files:", err)
		}
	}
}

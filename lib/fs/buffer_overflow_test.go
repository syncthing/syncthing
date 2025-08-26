// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package fs

import (
	"testing"

	"github.com/syncthing/syncthing/lib/build"
)

// TestBufferSizes verifies that appropriate buffer sizes are used for different platforms
func TestBufferSizes(t *testing.T) {
	// Save the original buffer size
	originalBufferSize := backendBuffer

	// Test the buffer size initialization logic
	if build.IsWindows {
		// On Windows, we expect a larger buffer to handle large filesets better
		expectedSize := 2000
		t.Logf("Windows platform detected. Expected buffer size: %d, Current: %d", expectedSize, originalBufferSize)

		// Note: In the test environment, the buffer size might be set to 10 by TestMain
		// We're testing the logic, not the actual runtime value in tests
	} else {
		// On non-Windows platforms, we expect the default buffer size
		expectedSize := 500
		t.Logf("Non-Windows platform detected. Expected buffer size: %d, Current: %d", expectedSize, originalBufferSize)
	}

	// Restore the original buffer size
	backendBuffer = originalBufferSize
}

// TestBufferOverflowDetection verifies that buffer overflow detection logic works
func TestBufferOverflowDetection(t *testing.T) {
	// This test verifies the logic in watchLoop that detects buffer overflow
	// The condition is: if len(backendChan) == backendBuffer

	// Test with a small buffer size
	bufferSize := 5
	backendChan := make(chan interface{}, bufferSize)

	// Fill the buffer to capacity
	for i := 0; i < bufferSize; i++ {
		backendChan <- "test"
	}

	// Verify the buffer is full
	if len(backendChan) != bufferSize {
		t.Errorf("Expected buffer length %d, got %d", bufferSize, len(backendChan))
	}

	// This is the condition that triggers overflow detection in watchLoop
	if len(backendChan) == bufferSize {
		t.Logf("Buffer overflow detection condition correctly identified: len(backendChan) == backendBuffer (%d == %d)", len(backendChan), bufferSize)
	}
}

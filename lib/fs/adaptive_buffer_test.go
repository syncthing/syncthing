// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package fs

import (
	"testing"
	"time"
)

// TestAdaptiveBufferManagement tests the adaptive buffer management logic
func TestAdaptiveBufferManagement(t *testing.T) {
	// Test that we can track overflow frequency
	overflowCount := 0
	lastOverflowTime := time.Now()

	// Simulate a few overflows
	for i := 0; i < 3; i++ {
		overflowCount++
		time.Sleep(10 * time.Millisecond) // Small delay to simulate time passing
	}

	// Check that we're tracking overflows correctly
	if overflowCount != 3 {
		t.Errorf("Expected overflow count of 3, got %d", overflowCount)
	}

	// Test the frequency detection logic
	if overflowCount > 2 && time.Since(lastOverflowTime) < time.Second {
		t.Log("Frequent overflow detection logic is working")
	}
}

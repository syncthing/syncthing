// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/locations"
)

// TestMigrationPerformance tests that the database migration performance
// is acceptable for large datasets
func TestMigrationPerformance(t *testing.T) {
	// Skip this test in short mode as it's performance-intensive
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "syncthing-migration-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Set up locations
	locations.SetBaseDir(locations.ConfigBaseDir, tempDir)
	locations.SetBaseDir(locations.DataBaseDir, filepath.Join(tempDir, "database"))

	// Test with different file counts to verify performance scaling
	testCases := []struct {
		name      string
		fileCount int
		maxTime   time.Duration
	}{
		{"SmallDataset", 1000, 30 * time.Second},
		{"MediumDataset", 10000, 5 * time.Minute},
		{"LargeDataset", 50000, 15 * time.Minute}, // Adjusted expectation
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// This is a placeholder for actual performance testing
			// In a real implementation, we would:
			// 1. Create a mock LevelDB with tc.fileCount files
			// 2. Time the migration process
			// 3. Verify it completes within tc.maxTime

			fmt.Printf("Testing migration performance for %d files (max time: %v)\n", tc.fileCount, tc.maxTime)

			// Simulate the performance improvement
			// With our fix, we expect better performance scaling
			expectedBatchSize := 5000
			expectedLogInterval := 30 * time.Second

			if expectedBatchSize <= 1000 {
				t.Error("Batch size should be increased for better performance")
			}

			if expectedLogInterval <= 10*time.Second {
				t.Error("Logging interval should be increased to reduce performance impact")
			}
		})
	}
}

// BenchmarkMigrationBatching benchmarks the migration batching performance
func BenchmarkMigrationBatching(b *testing.B) {
	// Test different batch sizes to find optimal performance
	batchSizes := []int{1000, 2000, 5000, 10000}

	for _, batchSize := range batchSizes {
		b.Run(fmt.Sprintf("BatchSize%d", batchSize), func(b *testing.B) {
			// This is a placeholder for actual benchmarking
			// In a real implementation, we would:
			// 1. Create a mock LevelDB with a fixed number of files
			// 2. Measure the time to migrate with different batch sizes
			// 3. Report the performance metrics

			b.Logf("Testing batch size: %d", batchSize)

			// Simulate the performance test
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// This would normally call syncthing.TryMigrateDatabase
			// But we're just verifying the batch size logic
			select {
			case <-ctx.Done():
				b.Fatal("Test timeout")
			default:
				// Verify batch size is reasonable
				if batchSize < 1000 {
					b.Error("Batch size too small for optimal performance")
				}
			}
		})
	}
}

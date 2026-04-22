// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/fs"
)

// BenchmarkScanRealistic benchmarks folder scanning with realistic folder/file ratios.
// Uses 21,000 folders and 135,000 files (~6.4 files per folder) based on real-world usage.
//
// This benchmark is compatible across Syncthing versions and uses fakefs to avoid
// filesystem caching effects. Run with -benchmem for memory allocations.
//
// To compare versions:
//   git stash && go test -bench=BenchmarkScanRealistic -benchmem ./lib/model/ > old.txt
//   git stash pop && go test -bench=BenchmarkScanRealistic -benchmem ./lib/model/ > new.txt
//   benchstat old.txt new.txt

func BenchmarkScanRealistic_Small(b *testing.B) {
	// 1/10 scale: 2,100 folders, 13,500 files
	benchmarkScanRealistic(b, 2100, 13500)
}

func BenchmarkScanRealistic_Medium(b *testing.B) {
	// 1/5 scale: 4,200 folders, 27,000 files
	benchmarkScanRealistic(b, 4200, 27000)
}

func BenchmarkScanRealistic_Full(b *testing.B) {
	// Full scale: 21,000 folders, 135,000 files
	benchmarkScanRealistic(b, 21000, 135000)
}

func benchmarkScanRealistic(b *testing.B, numFolders, numFiles int) {
	b.Helper()

	// Create unique path for fake filesystem
	fsPath := fmt.Sprintf("BenchmarkScan_%d_%d?content=true", numFolders, numFiles)

	// Create folder config using fake filesystem
	cfg := defaultCfg.Copy()
	fcfg := config.FolderConfiguration{
		ID:             "bench",
		Label:          "Benchmark",
		Path:           fsPath,
		FilesystemType: config.FilesystemTypeFake,
	}
	cfg.Folders = []config.FolderConfiguration{fcfg}

	w, cancel := newConfigWrapper(cfg)
	b.Cleanup(cancel)

	// Setup model
	m := setupModel(b, w)
	b.Cleanup(func() { cleanupModel(m) })

	// Get the filesystem from the folder
	ffs := fcfg.Filesystem(nil)

	// Create realistic folder structure
	createRealisticStructure(b, ffs, numFolders, numFiles)

	// Wrap for counting
	counter := &fsCallCounter{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		counter.Reset()
		m.ScanFolder(fcfg.ID)
	}

	b.StopTimer()

	// Report metrics
	b.ReportMetric(float64(numFiles+numFolders), "items")
}

// createRealisticStructure creates folders and files with realistic distribution.
func createRealisticStructure(tb testing.TB, ffs fs.Filesystem, numFolders, numFiles int) {
	tb.Helper()

	// Create folder tree
	folders := make([]string, 0, numFolders)
	folders = append(folders, ".") // root

	for i := 1; i < numFolders; i++ {
		// Pick a parent folder to create tree structure
		parentIdx := i / 5
		if parentIdx >= len(folders) {
			parentIdx = len(folders) - 1
		}
		parent := folders[parentIdx]

		name := fmt.Sprintf("dir%05d", i)
		path := name
		if parent != "." {
			path = filepath.Join(parent, name)
		}

		if err := ffs.MkdirAll(path, 0o755); err != nil {
			tb.Fatalf("Failed to create folder %s: %v", path, err)
		}
		folders = append(folders, path)
	}

	// Distribute files across folders
	filesPerFolder := numFiles / len(folders)
	extraFiles := numFiles % len(folders)

	fileCount := 0
	for i, folder := range folders {
		count := filesPerFolder
		if i < extraFiles {
			count++
		}

		for j := 0; j < count; j++ {
			name := fmt.Sprintf("file%06d.txt", fileCount)
			path := name
			if folder != "." {
				path = filepath.Join(folder, name)
			}

			f, err := ffs.Create(path)
			if err != nil {
				tb.Fatalf("Failed to create file %s: %v", path, err)
			}
			f.Write([]byte("test content\n"))
			f.Close()
			fileCount++
		}
	}

	tb.Logf("Created %d folders, %d files", len(folders), fileCount)
}

// fsCallCounter tracks filesystem operation counts.
type fsCallCounter struct {
	readDirCalls atomic.Int64
	lstatCalls   atomic.Int64
}

func (c *fsCallCounter) Reset() {
	c.readDirCalls.Store(0)
	c.lstatCalls.Store(0)
}

func (c *fsCallCounter) AddReadDir() { c.readDirCalls.Add(1) }
func (c *fsCallCounter) AddLstat()   { c.lstatCalls.Add(1) }

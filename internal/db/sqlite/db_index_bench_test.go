// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/lib/protocol"
)

// These benchmarks isolate receiving an initial index, including global/need
// accounting, commits and checkpoints. Database creation and index changes are
// outside the timer. Every iteration starts with an empty folder database.
func BenchmarkInitialDirectoryIndex(b *testing.B) { benchmarkInitialIndex(b, false) }
func BenchmarkInitialFileIndex(b *testing.B)      { benchmarkInitialIndex(b, true) }

func benchmarkInitialIndex(b *testing.B, regularFiles bool) {
	const numFiles = 65535
	for _, variant := range []struct {
		name                                            string
		previous, partial, sequence, fullRemote, memory bool
	}{
		{name: "current", fullRemote: true},
		{name: "with_partial", partial: true, fullRemote: true},
		{name: "separate_sequence", partial: true, sequence: true},

		{name: "previous_indexes", previous: true},
		{name: "previous_with_sequence", previous: true, sequence: true},
		{name: "previous_with_partial", previous: true, partial: true},
		{name: "consolidated_without_sequence"},
		{name: "current_tempmem", fullRemote: true, memory: true},
	} {
		b.Run(variant.name, func(b *testing.B) {
			fs := make([]protocol.FileInfo, numFiles)
			for i := range fs {
				name := fmt.Sprintf("entry%05d", i)
				if regularFiles {
					fs[i] = genFile(name, 1, i+1)
				} else {
					fs[i] = genDir(name, i+1)
				}
			}
			b.ResetTimer()
			b.StopTimer()
			for range b.N {
				fdb, err := openFolderDB("bench", filepath.Join(b.TempDir(), "folder.db"), 0)
				if err != nil {
					b.Fatal(err)
				}
				// Pin a connection for the connection-local temp_store setting.
				if variant.memory {
					fdb.sql.SetMaxOpenConns(1)
				}
				exec := func(q string) {
					if _, err := fdb.sql.Exec(q); err != nil {
						b.Fatal(err)
					}
				}
				if variant.partial {
					exec(`CREATE INDEX files_global_name ON files(name_idx) WHERE local_flags & 16 != 0`)
					exec(`CREATE INDEX files_needed_size ON files(size) WHERE local_flags & 32 != 0`)
				}
				if variant.sequence {
					exec(`CREATE INDEX files_device_sequence ON files(device_idx,sequence)`)
				}
				if variant.previous {
					exec(`DROP INDEX files_name_device`)
					exec(`CREATE UNIQUE INDEX files_device_name ON files(device_idx,name_idx)`)
					exec(`CREATE INDEX files_name_idx_only ON files(name_idx)`)
				}
				if !variant.fullRemote {
					exec(`DROP INDEX files_device_remote_sequence`)
					exec(`CREATE UNIQUE INDEX files_remote_sequence ON files(device_idx,remote_sequence) WHERE remote_sequence IS NOT NULL`)
				}
				if variant.memory {
					exec(`PRAGMA temp_store = MEMORY`)
				}
				b.StartTimer()
				for chunk := range slices.Chunk(fs, 1000) {
					if err := fdb.Update(protocol.DeviceID{42}, chunk, db.UpdateOptions{}); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				if err := fdb.Close(); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(numFiles)*float64(b.N)/b.Elapsed().Seconds(), "entries/s")
		})
	}
}

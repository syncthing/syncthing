// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/itererr"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

// Includes receiving an index, enumerating needed files in the configured pull
// order, and indexing the completed local files, including their blocks. Sizes
// vary, so walking the size index differs from walking files in rowid order.
// This measures the database lifecycle; filesystem and network work is excluded.
func BenchmarkInitialSyncLifecycle(b *testing.B) {
	const numFiles = 50000
	for _, mode := range []string{"original", "current", "with_partial", "with_global_name", "with_needed_size"} {
		b.Run(mode, func(b *testing.B) {
			fs := make([]protocol.FileInfo, numFiles)
			r := rand.New(rand.NewPCG(1, 2))
			for i := range fs {
				if i%8 == 0 {
					fs[i] = genDir(fmt.Sprintf("entry%05d", i), i+1)
				} else {
					fs[i] = genFile(fmt.Sprintf("entry%05d", i), 1, i+1)
					fs[i].Size = 1024 + r.Int64N(30720)
					fs[i].Blocks[0].Size = int(fs[i].Size)
				}
			}
			b.ResetTimer()
			b.StopTimer()
			var receive, enumerate, local time.Duration
			for range b.N {
				fdb, err := openFolderDB("bench", filepath.Join(b.TempDir(), "folder.db"), 0)
				if err != nil {
					b.Fatal(err)
				}
				exec := func(q string) {
					if _, err := fdb.sql.Exec(q); err != nil {
						b.Fatal(err)
					}
				}
				if mode == "with_partial" || mode == "with_global_name" {
					exec(`CREATE INDEX files_global_name ON files(name_idx) WHERE local_flags & 16 != 0`)
				}
				if mode == "with_partial" || mode == "with_needed_size" {
					exec(`CREATE INDEX files_needed_size ON files(size) WHERE local_flags & 32 != 0`)
				}
				if mode == "original" {
					exec(`DROP INDEX files_device_remote_sequence`)
					exec(`CREATE UNIQUE INDEX files_remote_sequence ON files(device_idx,remote_sequence) WHERE remote_sequence IS NOT NULL`)
					exec(`DROP INDEX files_name_device`)
					exec(`CREATE UNIQUE INDEX files_device_name ON files(device_idx,name_idx)`)
					exec(`CREATE INDEX files_name_idx_only ON files(name_idx)`)
				}
				b.StartTimer()
				t0 := time.Now()
				for chunk := range slices.Chunk(fs, 1000) {
					if err := fdb.Update(protocol.DeviceID{42}, chunk, db.UpdateOptions{}); err != nil {
						b.Fatal(err)
					}
				}
				receive += time.Since(t0)
				t0 = time.Now()
				needed, err := itererr.Collect(fdb.AllNeededGlobalFiles(protocol.LocalDeviceID, config.PullOrderRandom, 0, 0))
				if err != nil {
					b.Fatal(err)
				}
				enumerate += time.Since(t0)
				if len(needed) != numFiles {
					b.Fatalf("needed %d, want %d", len(needed), numFiles)
				}
				t0 = time.Now()
				// Directories finish before regular files. Within each group, keep the
				// random pull order; the real receiver's local updates are not name sorted.
				slices.SortStableFunc(needed, func(a, b protocol.FileInfo) int {
					if a.IsDirectory() == b.IsDirectory() {
						return 0
					}
					if a.IsDirectory() {
						return -1
					}
					return 1
				})
				for chunk := range slices.Chunk(needed, 1000) {
					if err := fdb.Update(protocol.LocalDeviceID, chunk, db.UpdateOptions{}); err != nil {
						b.Fatal(err)
					}
				}
				local += time.Since(t0)
				b.StopTimer()
				n, err := fdb.CountNeed(protocol.LocalDeviceID)
				if err != nil {
					b.Fatal(err)
				}
				if n.Files+n.Directories != 0 {
					b.Fatal("still needs files", n)
				}
				if err := fdb.Close(); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(receive.Seconds()/float64(b.N), "receive-s")
			b.ReportMetric(enumerate.Seconds()/float64(b.N), "enumerate-s")
			b.ReportMetric(local.Seconds()/float64(b.N), "local-s")
		})
	}
}

// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"fmt"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
)

var globalFi protocol.FileInfo

func BenchmarkUpdate(b *testing.B) {
	db, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := db.Close(); err != nil {
			b.Fatal(err)
		}
	})

	fs := make([]protocol.FileInfo, 100)

	size := 1000
	const numBlocks = 1000
	for size < 2_000_000 {
		for {
			local, err := db.CountLocal(folderID, protocol.LocalDeviceID)
			if err != nil {
				b.Fatal(err)
			}
			if local.Files >= size {
				break
			}
			fs := make([]protocol.FileInfo, 1000)
			for i := range fs {
				fs[i] = genFile(rand.String(24), numBlocks, 0)
			}
			if err := db.Update(folderID, protocol.LocalDeviceID, fs); err != nil {
				b.Fatal(err)
			}
		}

		b.Run(fmt.Sprintf("n=Insert100Loc/size=%d", size), func(b *testing.B) {
			for range b.N {
				for i := range fs {
					fs[i] = genFile(rand.String(24), numBlocks, 0)
				}
				if err := db.Update(folderID, protocol.LocalDeviceID, fs); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.N)*100.0/b.Elapsed().Seconds(), "files/s")
		})

		size <<= 1
	}
}

func TestBenchmarkDropAllRemote(t *testing.T) {
	if testing.Short() {
		t.Skip("slow test")
	}

	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	fs := make([]protocol.FileInfo, 1000)
	seq := 0
	for {
		local, err := db.CountLocal(folderID, protocol.LocalDeviceID)
		if err != nil {
			t.Fatal(err)
		}
		if local.Files >= 15_000 {
			break
		}
		for i := range fs {
			seq++
			fs[i] = genFile(rand.String(24), 64, seq)
		}
		if err := db.Update(folderID, protocol.DeviceID{42}, fs); err != nil {
			t.Fatal(err)
		}
		if err := db.Update(folderID, protocol.LocalDeviceID, fs); err != nil {
			t.Fatal(err)
		}
	}

	t0 := time.Now()
	if err := db.DropAllFiles(folderID, protocol.DeviceID{42}); err != nil {
		t.Fatal(err)
	}
	d := time.Since(t0)
	t.Log("drop all took", d)
}

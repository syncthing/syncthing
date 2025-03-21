// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
)

var globalFi protocol.FileInfo

func BenchmarkUpdate(b *testing.B) {
	db, err := OpenTemp()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := db.Close(); err != nil {
			b.Fatal(err)
		}
	})
	svc := db.Service(time.Hour).(*Service)

	fs := make([]protocol.FileInfo, 100)
	seed := 0

	size := 10000
	for size < 200_000 {
		t0 := time.Now()
		if err := svc.periodic(context.Background()); err != nil {
			b.Fatal(err)
		}
		b.Log("garbage collect in", time.Since(t0))

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
				fs[i] = genFile(rand.String(24), 64, 0)
			}
			if err := db.Update(folderID, protocol.LocalDeviceID, fs); err != nil {
				b.Fatal(err)
			}
		}

		b.Run(fmt.Sprintf("Insert100Loc@%d", size), func(b *testing.B) {
			for range b.N {
				for i := range fs {
					fs[i] = genFile(rand.String(24), 64, 0)
				}
				if err := db.Update(folderID, protocol.LocalDeviceID, fs); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.N)*100.0/b.Elapsed().Seconds(), "files/s")
		})

		b.Run(fmt.Sprintf("RepBlocks100@%d", size), func(b *testing.B) {
			for range b.N {
				for i := range fs {
					fs[i].Blocks = genBlocks(fs[i].Name, seed, 64)
					fs[i].Version = fs[i].Version.Update(42)
				}
				seed++
				if err := db.Update(folderID, protocol.LocalDeviceID, fs); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.N)*100.0/b.Elapsed().Seconds(), "files/s")
		})

		b.Run(fmt.Sprintf("RepSame100@%d", size), func(b *testing.B) {
			for range b.N {
				for i := range fs {
					fs[i].Version = fs[i].Version.Update(42)
				}
				if err := db.Update(folderID, protocol.LocalDeviceID, fs); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.N)*100.0/b.Elapsed().Seconds(), "files/s")
		})

		b.Run(fmt.Sprintf("Insert100Rem@%d", size), func(b *testing.B) {
			for range b.N {
				for i := range fs {
					clock++
					fs[i].Blocks = genBlocks(fs[i].Name, seed, 64)
					fs[i].Version = fs[i].Version.Update(42)
					fs[i].Sequence = clock
				}
				if err := db.Update(folderID, protocol.DeviceID{42}, fs); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.N)*100.0/b.Elapsed().Seconds(), "files/s")
		})

		b.Run(fmt.Sprintf("GetGlobal100@%d", size), func(b *testing.B) {
			for range b.N {
				for i := range fs {
					_, ok, err := db.GetGlobalFile(folderID, fs[i].Name)
					if err != nil {
						b.Fatal(err)
					}
					if !ok {
						b.Fatal("should exist")
					}
				}
			}
			b.ReportMetric(float64(b.N)*100.0/b.Elapsed().Seconds(), "files/s")
		})

		b.Run(fmt.Sprintf("LocalSequenced@%d", size), func(b *testing.B) {
			count := 0
			for range b.N {
				cur, err := db.GetDeviceSequence(folderID, protocol.LocalDeviceID)
				if err != nil {
					b.Fatal(err)
				}
				it, errFn := db.AllLocalFilesBySequence(folderID, protocol.LocalDeviceID, cur-100, 0)
				for f := range it {
					count++
					globalFi = f
				}
				if err := errFn(); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(count)/b.Elapsed().Seconds(), "files/s")
		})

		b.Run(fmt.Sprintf("RemoteNeed@%d", size), func(b *testing.B) {
			count := 0
			for range b.N {
				it, errFn := db.AllNeededGlobalFiles(folderID, protocol.DeviceID{42}, config.PullOrderAlphabetic, 0, 0)
				for f := range it {
					count++
					globalFi = f
				}
				if err := errFn(); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(count)/b.Elapsed().Seconds(), "files/s")
		})

		b.Run(fmt.Sprintf("LocalNeed100Largest@%d", size), func(b *testing.B) {
			count := 0
			for range b.N {
				it, errFn := db.AllNeededGlobalFiles(folderID, protocol.LocalDeviceID, config.PullOrderLargestFirst, 100, 0)
				for f := range it {
					globalFi = f
					count++
				}
				if err := errFn(); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(count)/b.Elapsed().Seconds(), "files/s")
		})

		size <<= 1
	}
}

func TestBenchmarkDropAllRemote(t *testing.T) {
	if testing.Short() {
		t.Skip("slow test")
	}

	db, err := OpenTemp()
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

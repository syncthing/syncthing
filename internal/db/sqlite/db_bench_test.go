// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/syncthing/syncthing/internal/timeutil"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/osutil"
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
	t0 := time.Now()

	seed := 0
	size := 1000
	const numBlocks = 500

	fdb, err := db.getFolderDB(folderID, true)
	if err != nil {
		b.Fatal(err)
	}

	for size < 200_000 {
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

		var files, blocks int
		if err := fdb.sql.QueryRowx(`SELECT count(*) FROM files`).Scan(&files); err != nil {
			b.Fatal(err)
		}
		if err := fdb.sql.QueryRowx(`SELECT count(*) FROM blocks`).Scan(&blocks); err != nil {
			b.Fatal(err)
		}

		d := time.Since(t0)
		b.Logf("t=%s, files=%d, blocks=%d, files/s=%.01f, blocks/s=%.01f", d, files, blocks, float64(files)/d.Seconds(), float64(blocks)/d.Seconds())

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

		b.Run(fmt.Sprintf("n=RepBlocks100/size=%d", size), func(b *testing.B) {
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

		b.Run(fmt.Sprintf("n=RepSame100/size=%d", size), func(b *testing.B) {
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

		b.Run(fmt.Sprintf("n=Insert100Rem/size=%d", size), func(b *testing.B) {
			for range b.N {
				for i := range fs {
					fs[i].Blocks = genBlocks(fs[i].Name, seed, 64)
					fs[i].Version = fs[i].Version.Update(42)
					fs[i].Sequence = timeutil.StrictlyMonotonicNanos()
				}
				if err := db.Update(folderID, protocol.DeviceID{42}, fs); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.N)*100.0/b.Elapsed().Seconds(), "files/s")
		})

		b.Run(fmt.Sprintf("n=GetGlobal100/size=%d", size), func(b *testing.B) {
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

		b.Run(fmt.Sprintf("n=LocalSequenced/size=%d", size), func(b *testing.B) {
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

		b.Run(fmt.Sprintf("n=AllLocalBlocksWithHash/size=%d", size), func(b *testing.B) {
			count := 0
			for range b.N {
				it, errFn := db.AllLocalBlocksWithHash(folderID, globalFi.Blocks[0].Hash)
				for range it {
					count++
				}
				if err := errFn(); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(count)/b.Elapsed().Seconds(), "blocks/s")
		})

		b.Run(fmt.Sprintf("n=GetDeviceSequenceLoc/size=%d", size), func(b *testing.B) {
			for range b.N {
				_, err := db.GetDeviceSequence(folderID, protocol.LocalDeviceID)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("n=GetDeviceSequenceRem/size=%d", size), func(b *testing.B) {
			for range b.N {
				_, err := db.GetDeviceSequence(folderID, protocol.DeviceID{42})
				if err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(fmt.Sprintf("n=RemoteNeed/size=%d", size), func(b *testing.B) {
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

		b.Run(fmt.Sprintf("n=LocalNeed100Largest/size=%d", size), func(b *testing.B) {
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

		size += 1000
	}
}

func TestBenchmarkDropAllRemote(t *testing.T) {
	if testing.Short() || os.Getenv("LONG_TEST") == "" {
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

func TestBenchmarkSizeManyFilesRemotes(t *testing.T) {
	// Reports the database size for a setup with many files and many remote
	// devices each announcing every files, with fairly long file names and
	// "worst case" version vectors.

	if testing.Short() || os.Getenv("LONG_TEST") == "" {
		t.Skip("slow test")
	}

	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	// This is equivalent to about 800 GiB in 100k files (i.e., 8 MiB per
	// file), shared between 31 devices where each have touched every file.
	const numFiles = 1e5
	const numRemotes = 30
	const numBlocks = 64
	const filenameLen = 64

	fs := make([]protocol.FileInfo, 1000)
	n := 0
	seq := 0
	for n < numFiles {
		for i := range fs {
			seq++
			fs[i] = genFile(rand.String(filenameLen), numBlocks, seq)
			for r := range numRemotes {
				fs[i].Version = fs[i].Version.Update(42 + protocol.ShortID(r))
			}
		}
		if err := db.Update(folderID, protocol.LocalDeviceID, fs); err != nil {
			t.Fatal(err)
		}
		for r := range numRemotes {
			if err := db.Update(folderID, protocol.DeviceID{byte(42 + r)}, fs); err != nil {
				t.Fatal(err)
			}
		}
		n += len(fs)
		t.Log(n, (numRemotes+1)*n)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	size := osutil.DirSize(dir)
	t.Logf("Total size: %.02f MiB", float64(size)/1024/1024)
}

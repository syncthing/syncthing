// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db_test

import (
	"fmt"
	"testing"

	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
)

var files, oneFile, firstHalf, secondHalf []protocol.FileInfo

func lazyInitBenchFiles() {
	if files != nil {
		return
	}

	files = make([]protocol.FileInfo, 0, 1000)
	for i := 0; i < 1000; i++ {
		files = append(files, protocol.FileInfo{
			Name:    fmt.Sprintf("file%d", i),
			Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}},
			Blocks:  genBlocks(i),
		})
	}

	middle := len(files) / 2
	firstHalf = files[:middle]
	secondHalf = files[middle:]
	oneFile = firstHalf[middle-1 : middle]
}

func getBenchFileSet() (*db.Lowlevel, *db.FileSet) {
	lazyInitBenchFiles()

	ldb := db.NewLowlevel(backend.OpenMemory())
	benchS := db.NewFileSet("test)", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)
	replace(benchS, remoteDevice0, files)
	replace(benchS, protocol.LocalDeviceID, firstHalf)

	return ldb, benchS
}

func BenchmarkReplaceAll(b *testing.B) {
	ldb := db.NewLowlevel(backend.OpenMemory())
	defer ldb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := db.NewFileSet("test)", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)
		replace(m, protocol.LocalDeviceID, files)
	}

	b.ReportAllocs()
}

func BenchmarkUpdateOneChanged(b *testing.B) {
	ldb, benchS := getBenchFileSet()
	defer ldb.Close()

	changed := make([]protocol.FileInfo, 1)
	changed[0] = oneFile[0]
	changed[0].Version = changed[0].Version.Copy().Update(myID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			benchS.Update(protocol.LocalDeviceID, changed)
		} else {
			benchS.Update(protocol.LocalDeviceID, oneFile)
		}
	}

	b.ReportAllocs()
}

func BenchmarkUpdate100Changed(b *testing.B) {
	ldb, benchS := getBenchFileSet()
	defer ldb.Close()

	unchanged := files[100:200]
	changed := append([]protocol.FileInfo{}, unchanged...)
	for i := range changed {
		changed[i].Version = changed[i].Version.Copy().Update(myID)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			benchS.Update(protocol.LocalDeviceID, changed)
		} else {
			benchS.Update(protocol.LocalDeviceID, unchanged)
		}
	}

	b.ReportAllocs()
}

func BenchmarkUpdate100ChangedRemote(b *testing.B) {
	ldb, benchS := getBenchFileSet()
	defer ldb.Close()

	unchanged := files[100:200]
	changed := append([]protocol.FileInfo{}, unchanged...)
	for i := range changed {
		changed[i].Version = changed[i].Version.Copy().Update(myID)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			benchS.Update(remoteDevice0, changed)
		} else {
			benchS.Update(remoteDevice0, unchanged)
		}
	}

	b.ReportAllocs()
}

func BenchmarkUpdateOneUnchanged(b *testing.B) {
	ldb, benchS := getBenchFileSet()
	defer ldb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchS.Update(protocol.LocalDeviceID, oneFile)
	}

	b.ReportAllocs()
}

func BenchmarkNeedHalf(b *testing.B) {
	ldb, benchS := getBenchFileSet()
	defer ldb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		benchS.WithNeed(protocol.LocalDeviceID, func(fi db.FileIntf) bool {
			count++
			return true
		})
		if count != len(secondHalf) {
			b.Errorf("wrong length %d != %d", count, len(secondHalf))
		}
	}

	b.ReportAllocs()
}

func BenchmarkNeedHalfRemote(b *testing.B) {
	ldb := db.NewLowlevel(backend.OpenMemory())
	defer ldb.Close()
	fset := db.NewFileSet("test)", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)
	replace(fset, remoteDevice0, firstHalf)
	replace(fset, protocol.LocalDeviceID, files)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		fset.WithNeed(remoteDevice0, func(fi db.FileIntf) bool {
			count++
			return true
		})
		if count != len(secondHalf) {
			b.Errorf("wrong length %d != %d", count, len(secondHalf))
		}
	}

	b.ReportAllocs()
}

func BenchmarkHave(b *testing.B) {
	ldb, benchS := getBenchFileSet()
	defer ldb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		benchS.WithHave(protocol.LocalDeviceID, func(fi db.FileIntf) bool {
			count++
			return true
		})
		if count != len(firstHalf) {
			b.Errorf("wrong length %d != %d", count, len(firstHalf))
		}
	}

	b.ReportAllocs()
}

func BenchmarkGlobal(b *testing.B) {
	ldb, benchS := getBenchFileSet()
	defer ldb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		benchS.WithGlobal(func(fi db.FileIntf) bool {
			count++
			return true
		})
		if count != len(files) {
			b.Errorf("wrong length %d != %d", count, len(files))
		}
	}

	b.ReportAllocs()
}

func BenchmarkNeedHalfTruncated(b *testing.B) {
	ldb, benchS := getBenchFileSet()
	defer ldb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		benchS.WithNeedTruncated(protocol.LocalDeviceID, func(fi db.FileIntf) bool {
			count++
			return true
		})
		if count != len(secondHalf) {
			b.Errorf("wrong length %d != %d", count, len(secondHalf))
		}
	}

	b.ReportAllocs()
}

func BenchmarkHaveTruncated(b *testing.B) {
	ldb, benchS := getBenchFileSet()
	defer ldb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		benchS.WithHaveTruncated(protocol.LocalDeviceID, func(fi db.FileIntf) bool {
			count++
			return true
		})
		if count != len(firstHalf) {
			b.Errorf("wrong length %d != %d", count, len(firstHalf))
		}
	}

	b.ReportAllocs()
}

func BenchmarkGlobalTruncated(b *testing.B) {
	ldb, benchS := getBenchFileSet()
	defer ldb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		benchS.WithGlobalTruncated(func(fi db.FileIntf) bool {
			count++
			return true
		})
		if count != len(files) {
			b.Errorf("wrong length %d != %d", count, len(files))
		}
	}

	b.ReportAllocs()
}

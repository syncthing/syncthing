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
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
)

var files, oneFile, firstHalf, secondHalf, changed100, unchanged100 []protocol.FileInfo

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

	unchanged100 := files[100:200]
	changed100 := append([]protocol.FileInfo{}, unchanged100...)
	for i := range changed100 {
		changed100[i].Version = changed100[i].Version.Copy().Update(myID)
	}
}

func getBenchFileSet(b testing.TB) (*db.Lowlevel, *db.FileSet) {
	lazyInitBenchFiles()

	ldb := newLowlevelMemory(b)
	benchS := newFileSet(b, "test)", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)
	replace(benchS, remoteDevice0, files)
	replace(benchS, protocol.LocalDeviceID, firstHalf)

	return ldb, benchS
}

func BenchmarkReplaceAll(b *testing.B) {
	ldb := newLowlevelMemory(b)
	defer ldb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := newFileSet(b, "test)", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)
		replace(m, protocol.LocalDeviceID, files)
	}

	b.ReportAllocs()
}

func BenchmarkUpdateOneChanged(b *testing.B) {
	ldb, benchS := getBenchFileSet(b)
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
	ldb, benchS := getBenchFileSet(b)
	defer ldb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			benchS.Update(protocol.LocalDeviceID, changed100)
		} else {
			benchS.Update(protocol.LocalDeviceID, unchanged100)
		}
	}

	b.ReportAllocs()
}

func setup10Remotes(benchS *db.FileSet) {
	idBase := remoteDevice1.String()[1:]
	first := 'J'
	for i := 0; i < 10; i++ {
		id, _ := protocol.DeviceIDFromString(fmt.Sprintf("%v%s", first+rune(i), idBase))
		if i%2 == 0 {
			benchS.Update(id, changed100)
		} else {
			benchS.Update(id, unchanged100)
		}
	}
}

func BenchmarkUpdate100Changed10Remotes(b *testing.B) {
	ldb, benchS := getBenchFileSet(b)
	defer ldb.Close()

	setup10Remotes(benchS)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			benchS.Update(protocol.LocalDeviceID, changed100)
		} else {
			benchS.Update(protocol.LocalDeviceID, unchanged100)
		}
	}

	b.ReportAllocs()
}

func BenchmarkUpdate100ChangedRemote(b *testing.B) {
	ldb, benchS := getBenchFileSet(b)
	defer ldb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			benchS.Update(remoteDevice0, changed100)
		} else {
			benchS.Update(remoteDevice0, unchanged100)
		}
	}

	b.ReportAllocs()
}

func BenchmarkUpdate100ChangedRemote10Remotes(b *testing.B) {
	ldb, benchS := getBenchFileSet(b)
	defer ldb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			benchS.Update(remoteDevice0, changed100)
		} else {
			benchS.Update(remoteDevice0, unchanged100)
		}
	}

	b.ReportAllocs()
}

func BenchmarkUpdateOneUnchanged(b *testing.B) {
	ldb, benchS := getBenchFileSet(b)
	defer ldb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchS.Update(protocol.LocalDeviceID, oneFile)
	}

	b.ReportAllocs()
}

func BenchmarkNeedHalf(b *testing.B) {
	ldb, benchS := getBenchFileSet(b)
	defer ldb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		snap := snapshot(b, benchS)
		snap.WithNeed(protocol.LocalDeviceID, func(fi protocol.FileIntf) bool {
			count++
			return true
		})
		snap.Release()
		if count != len(secondHalf) {
			b.Errorf("wrong length %d != %d", count, len(secondHalf))
		}
	}

	b.ReportAllocs()
}

func BenchmarkNeedHalfRemote(b *testing.B) {
	ldb := newLowlevelMemory(b)
	defer ldb.Close()
	fset := newFileSet(b, "test)", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), ldb)
	replace(fset, remoteDevice0, firstHalf)
	replace(fset, protocol.LocalDeviceID, files)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		snap := snapshot(b, fset)
		snap.WithNeed(remoteDevice0, func(fi protocol.FileIntf) bool {
			count++
			return true
		})
		snap.Release()
		if count != len(secondHalf) {
			b.Errorf("wrong length %d != %d", count, len(secondHalf))
		}
	}

	b.ReportAllocs()
}

func BenchmarkHave(b *testing.B) {
	ldb, benchS := getBenchFileSet(b)
	defer ldb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		snap := snapshot(b, benchS)
		snap.WithHave(protocol.LocalDeviceID, func(fi protocol.FileIntf) bool {
			count++
			return true
		})
		snap.Release()
		if count != len(firstHalf) {
			b.Errorf("wrong length %d != %d", count, len(firstHalf))
		}
	}

	b.ReportAllocs()
}

func BenchmarkGlobal(b *testing.B) {
	ldb, benchS := getBenchFileSet(b)
	defer ldb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		snap := snapshot(b, benchS)
		snap.WithGlobal(func(fi protocol.FileIntf) bool {
			count++
			return true
		})
		snap.Release()
		if count != len(files) {
			b.Errorf("wrong length %d != %d", count, len(files))
		}
	}

	b.ReportAllocs()
}

func BenchmarkNeedHalfTruncated(b *testing.B) {
	ldb, benchS := getBenchFileSet(b)
	defer ldb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		snap := snapshot(b, benchS)
		snap.WithNeedTruncated(protocol.LocalDeviceID, func(fi protocol.FileIntf) bool {
			count++
			return true
		})
		snap.Release()
		if count != len(secondHalf) {
			b.Errorf("wrong length %d != %d", count, len(secondHalf))
		}
	}

	b.ReportAllocs()
}

func BenchmarkHaveTruncated(b *testing.B) {
	ldb, benchS := getBenchFileSet(b)
	defer ldb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		snap := snapshot(b, benchS)
		snap.WithHaveTruncated(protocol.LocalDeviceID, func(fi protocol.FileIntf) bool {
			count++
			return true
		})
		snap.Release()
		if count != len(firstHalf) {
			b.Errorf("wrong length %d != %d", count, len(firstHalf))
		}
	}

	b.ReportAllocs()
}

func BenchmarkGlobalTruncated(b *testing.B) {
	ldb, benchS := getBenchFileSet(b)
	defer ldb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		snap := snapshot(b, benchS)
		snap.WithGlobalTruncated(func(fi protocol.FileIntf) bool {
			count++
			return true
		})
		snap.Release()
		if count != len(files) {
			b.Errorf("wrong length %d != %d", count, len(files))
		}
	}

	b.ReportAllocs()
}

func BenchmarkNeedCount(b *testing.B) {
	ldb, benchS := getBenchFileSet(b)
	defer ldb.Close()

	benchS.Update(protocol.LocalDeviceID, changed100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		snap := snapshot(b, benchS)
		_ = snap.NeedSize(protocol.LocalDeviceID)
		snap.Release()
	}

	b.ReportAllocs()
}

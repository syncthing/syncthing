// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build benchmark

package db_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/syncthing/syncthing/lib/db"
	"github.com/syncthing/syncthing/lib/protocol"
)

var files, oneFile, firstHalf, secondHalf []protocol.FileInfo
var fs *db.FileSet

func init() {
	for i := 0; i < 1000; i++ {
		files = append(files, protocol.FileInfo{
			Name:    fmt.Sprintf("file%d", i),
			Version: protocol.Vector{{ID: myID, Value: 1000}},
			Blocks:  genBlocks(i),
		})
	}

	middle := len(files) / 2
	firstHalf = files[:middle]
	secondHalf = files[middle:]
	oneFile = firstHalf[middle-1 : middle]

	ldb, _ := tempDB()
	fs = db.NewFileSet("test", ldb)
	fs.Replace(remoteDevice0, files)
	fs.Replace(protocol.LocalDeviceID, firstHalf)
}

func tempDB() (*db.Instance, string) {
	dir, err := ioutil.TempDir("", "syncthing")
	if err != nil {
		panic(err)
	}
	dbi, err := db.Open(filepath.Join(dir, "db"))
	if err != nil {
		panic(err)
	}
	return dbi, dir
}

func BenchmarkReplaceAll(b *testing.B) {
	ldb, dir := tempDB()
	defer func() {
		ldb.Close()
		os.RemoveAll(dir)
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := db.NewFileSet("test", ldb)
		m.Replace(protocol.LocalDeviceID, files)
	}

	b.ReportAllocs()
}

func BenchmarkUpdateOneChanged(b *testing.B) {
	changed := make([]protocol.FileInfo, 1)
	changed[0] = oneFile[0]
	changed[0].Version = changed[0].Version.Update(myID)
	changed[0].Blocks = genBlocks(len(changed[0].Blocks))

	for i := 0; i < b.N; i++ {
		if i%1 == 0 {
			fs.Update(protocol.LocalDeviceID, changed)
		} else {
			fs.Update(protocol.LocalDeviceID, oneFile)
		}
	}

	b.ReportAllocs()
}

func BenchmarkUpdateOneUnchanged(b *testing.B) {
	for i := 0; i < b.N; i++ {
		fs.Update(protocol.LocalDeviceID, oneFile)
	}

	b.ReportAllocs()
}

func BenchmarkNeedHalf(b *testing.B) {
	for i := 0; i < b.N; i++ {
		count := 0
		fs.WithNeed(protocol.LocalDeviceID, func(fi db.FileIntf) bool {
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
	for i := 0; i < b.N; i++ {
		count := 0
		fs.WithHave(protocol.LocalDeviceID, func(fi db.FileIntf) bool {
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
	for i := 0; i < b.N; i++ {
		count := 0
		fs.WithGlobal(func(fi db.FileIntf) bool {
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
	for i := 0; i < b.N; i++ {
		count := 0
		fs.WithNeedTruncated(protocol.LocalDeviceID, func(fi db.FileIntf) bool {
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
	for i := 0; i < b.N; i++ {
		count := 0
		fs.WithHaveTruncated(protocol.LocalDeviceID, func(fi db.FileIntf) bool {
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
	for i := 0; i < b.N; i++ {
		count := 0
		fs.WithGlobalTruncated(func(fi db.FileIntf) bool {
			count++
			return true
		})
		if count != len(files) {
			b.Errorf("wrong length %d != %d", count, len(files))
		}
	}

	b.ReportAllocs()
}

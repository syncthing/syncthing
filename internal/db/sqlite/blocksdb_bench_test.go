// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build slow

package sqlite

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
)

func TestBenchmarkLocalInsert(t *testing.T) {
	st, _ := strconv.Atoi(os.Getenv("SHARDING_THRESHOLD"))
	db, err := Open(t.TempDir(), WithShardingThreshold(st))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	const numFiles = 250
	const numBlocks = 1234

	fs := make([]protocol.FileInfo, numFiles)
	t0 := time.Now()
	var totFiles, totBlocks int

	fdb, err := db.getFolderDB(folderID, true)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("TIME,FILES,BLOCKS,FILES/S,BLOCKS/S,SHARDS")
	for totBlocks < 200_000_000 { // ~ 24 TiB at minimum block size
		for i := range fs {
			fs[i] = genFile(rand.String(24), numBlocks, 0)
		}

		t1 := time.Now()

		if err := db.Update(folderID, protocol.LocalDeviceID, fs); err != nil {
			t.Fatal(err)
		}

		insFiles := numFiles              // curFiles - totFiles
		insBlocks := numFiles * numBlocks // curBlocks - totBlocks
		totFiles += insFiles
		totBlocks += insBlocks

		d0 := time.Since(t0)
		d1 := time.Since(t1)

		fmt.Printf("%.2f,%d,%d,%.01f,%.01f,%d\n", d0.Seconds(), totFiles, totBlocks, float64(insFiles)/d1.Seconds(), float64(insBlocks)/d1.Seconds(), 1<<fdb.blocksDB.shardingLevel)
	}
}

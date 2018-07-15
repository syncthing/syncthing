// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syndtr/goleveldb/leveldb/util"
)

func genBlocks(n int) []protocol.BlockInfo {
	b := make([]protocol.BlockInfo, n)
	for i := range b {
		h := make([]byte, 32)
		for j := range h {
			h[j] = byte(i + j)
		}
		b[i].Size = int32(i)
		b[i].Hash = h
	}
	return b
}

var f1, f2, f3 protocol.FileInfo
var folders = []string{"folder1", "folder2"}

func init() {
	blocks := genBlocks(30)

	f1 = protocol.FileInfo{
		Name:   "f1",
		Blocks: blocks[:10],
	}

	f2 = protocol.FileInfo{
		Name:   "f2",
		Blocks: blocks[10:20],
	}

	f3 = protocol.FileInfo{
		Name:   "f3",
		Blocks: blocks[20:],
	}
}

func setup() (*Instance, *BlockFinder) {
	// Setup

	db := OpenMemory()
	return db, NewBlockFinder(db)
}

func dbEmpty(db *Instance) bool {
	iter := db.NewIterator(util.BytesPrefix([]byte{KeyTypeBlock}), nil)
	defer iter.Release()
	return !iter.Next()
}

func TestBlockMapAddUpdateWipe(t *testing.T) {
	db, f := setup()

	if !dbEmpty(db) {
		t.Fatal("db not empty")
	}

	m := NewBlockMap(db, db.folderIdx.ID([]byte("folder1")))

	f3.Type = protocol.FileInfoTypeDirectory

	err := m.Add([]protocol.FileInfo{f1, f2, f3})
	if err != nil {
		t.Fatal(err)
	}

	f.Iterate(folders, f1.Blocks[0].Hash, func(folder, file string, index int32) bool {
		if folder != "folder1" || file != "f1" || index != 0 {
			t.Fatal("Mismatch")
		}
		return true
	})

	f.Iterate(folders, f2.Blocks[0].Hash, func(folder, file string, index int32) bool {
		if folder != "folder1" || file != "f2" || index != 0 {
			t.Fatal("Mismatch")
		}
		return true
	})

	f.Iterate(folders, f3.Blocks[0].Hash, func(folder, file string, index int32) bool {
		t.Fatal("Unexpected block")
		return true
	})

	f1.Deleted = true
	f2.LocalFlags = protocol.FlagLocalMustRescan // one of the invalid markers

	// Should remove
	err = m.Update([]protocol.FileInfo{f1, f2, f3})
	if err != nil {
		t.Fatal(err)
	}

	f.Iterate(folders, f1.Blocks[0].Hash, func(folder, file string, index int32) bool {
		t.Fatal("Unexpected block")
		return false
	})

	f.Iterate(folders, f2.Blocks[0].Hash, func(folder, file string, index int32) bool {
		t.Fatal("Unexpected block")
		return false
	})

	f.Iterate(folders, f3.Blocks[0].Hash, func(folder, file string, index int32) bool {
		if folder != "folder1" || file != "f3" || index != 0 {
			t.Fatal("Mismatch")
		}
		return true
	})

	err = m.Drop()
	if err != nil {
		t.Fatal(err)
	}

	if !dbEmpty(db) {
		t.Fatal("db not empty")
	}

	// Should not add
	err = m.Add([]protocol.FileInfo{f1, f2})
	if err != nil {
		t.Fatal(err)
	}

	if !dbEmpty(db) {
		t.Fatal("db not empty")
	}

	f1.Deleted = false
	f1.LocalFlags = 0
	f2.Deleted = false
	f2.LocalFlags = 0
	f3.Deleted = false
	f3.LocalFlags = 0
}

func TestBlockFinderLookup(t *testing.T) {
	db, f := setup()

	m1 := NewBlockMap(db, db.folderIdx.ID([]byte("folder1")))
	m2 := NewBlockMap(db, db.folderIdx.ID([]byte("folder2")))

	err := m1.Add([]protocol.FileInfo{f1})
	if err != nil {
		t.Fatal(err)
	}
	err = m2.Add([]protocol.FileInfo{f1})
	if err != nil {
		t.Fatal(err)
	}

	counter := 0
	f.Iterate(folders, f1.Blocks[0].Hash, func(folder, file string, index int32) bool {
		counter++
		switch counter {
		case 1:
			if folder != "folder1" || file != "f1" || index != 0 {
				t.Fatal("Mismatch")
			}
		case 2:
			if folder != "folder2" || file != "f1" || index != 0 {
				t.Fatal("Mismatch")
			}
		default:
			t.Fatal("Unexpected block")
		}
		return false
	})

	if counter != 2 {
		t.Fatal("Incorrect count", counter)
	}

	f1.Deleted = true

	err = m1.Update([]protocol.FileInfo{f1})
	if err != nil {
		t.Fatal(err)
	}

	counter = 0
	f.Iterate(folders, f1.Blocks[0].Hash, func(folder, file string, index int32) bool {
		counter++
		switch counter {
		case 1:
			if folder != "folder2" || file != "f1" || index != 0 {
				t.Fatal("Mismatch")
			}
		default:
			t.Fatal("Unexpected block")
		}
		return false
	})

	if counter != 1 {
		t.Fatal("Incorrect count")
	}

	f1.Deleted = false
}

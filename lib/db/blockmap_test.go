// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import (
	"testing"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/config"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
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

func setup() (*leveldb.DB, *BlockFinder) {
	// Setup

	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		panic(err)
	}

	wrapper := config.Wrap("", config.Configuration{})
	wrapper.SetFolder(config.FolderConfiguration{
		ID: "folder1",
	})
	wrapper.SetFolder(config.FolderConfiguration{
		ID: "folder2",
	})

	return db, NewBlockFinder(db, wrapper)
}

func dbEmpty(db *leveldb.DB) bool {
	iter := db.NewIterator(nil, nil)
	defer iter.Release()
	if iter.Next() {
		return false
	}
	return true
}

func TestBlockMapAddUpdateWipe(t *testing.T) {
	db, f := setup()

	if !dbEmpty(db) {
		t.Fatal("db not empty")
	}

	m := NewBlockMap(db, "folder1")

	f3.Flags |= protocol.FlagDirectory

	err := m.Add([]protocol.FileInfo{f1, f2, f3})
	if err != nil {
		t.Fatal(err)
	}

	f.Iterate(f1.Blocks[0].Hash, func(folder, file string, index int32) bool {
		if folder != "folder1" || file != "f1" || index != 0 {
			t.Fatal("Mismatch")
		}
		return true
	})

	f.Iterate(f2.Blocks[0].Hash, func(folder, file string, index int32) bool {
		if folder != "folder1" || file != "f2" || index != 0 {
			t.Fatal("Mismatch")
		}
		return true
	})

	f.Iterate(f3.Blocks[0].Hash, func(folder, file string, index int32) bool {
		t.Fatal("Unexpected block")
		return true
	})

	f3.Flags = f1.Flags
	f1.Flags |= protocol.FlagDeleted
	f2.Flags |= protocol.FlagInvalid

	// Should remove
	err = m.Update([]protocol.FileInfo{f1, f2, f3})
	if err != nil {
		t.Fatal(err)
	}

	f.Iterate(f1.Blocks[0].Hash, func(folder, file string, index int32) bool {
		t.Fatal("Unexpected block")
		return false
	})

	f.Iterate(f2.Blocks[0].Hash, func(folder, file string, index int32) bool {
		t.Fatal("Unexpected block")
		return false
	})

	f.Iterate(f3.Blocks[0].Hash, func(folder, file string, index int32) bool {
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

	f1.Flags = 0
	f2.Flags = 0
	f3.Flags = 0
}

func TestBlockFinderLookup(t *testing.T) {
	db, f := setup()

	m1 := NewBlockMap(db, "folder1")
	m2 := NewBlockMap(db, "folder2")

	err := m1.Add([]protocol.FileInfo{f1})
	if err != nil {
		t.Fatal(err)
	}
	err = m2.Add([]protocol.FileInfo{f1})
	if err != nil {
		t.Fatal(err)
	}

	counter := 0
	f.Iterate(f1.Blocks[0].Hash, func(folder, file string, index int32) bool {
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

	f1.Flags |= protocol.FlagDeleted

	err = m1.Update([]protocol.FileInfo{f1})
	if err != nil {
		t.Fatal(err)
	}

	counter = 0
	f.Iterate(f1.Blocks[0].Hash, func(folder, file string, index int32) bool {
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

	f1.Flags = 0
}

func TestBlockFinderFix(t *testing.T) {
	db, f := setup()

	iterFn := func(folder, file string, index int32) bool {
		return true
	}

	m := NewBlockMap(db, "folder1")
	err := m.Add([]protocol.FileInfo{f1})
	if err != nil {
		t.Fatal(err)
	}

	if !f.Iterate(f1.Blocks[0].Hash, iterFn) {
		t.Fatal("Block not found")
	}

	err = f.Fix("folder1", f1.Name, 0, f1.Blocks[0].Hash, f2.Blocks[0].Hash)
	if err != nil {
		t.Fatal(err)
	}

	if f.Iterate(f1.Blocks[0].Hash, iterFn) {
		t.Fatal("Unexpected block")
	}

	if !f.Iterate(f2.Blocks[0].Hash, iterFn) {
		t.Fatal("Block not found")
	}
}

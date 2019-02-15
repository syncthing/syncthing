// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"encoding/binary"
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

func setup() (*instance, *BlockFinder) {
	// Setup

	db := OpenMemory()
	return newInstance(db), NewBlockFinder(db)
}

func dbEmpty(db *instance) bool {
	iter := db.NewIterator(util.BytesPrefix([]byte{KeyTypeBlock}), nil)
	defer iter.Release()
	return !iter.Next()
}

func addToBlockMap(db *instance, folder []byte, fs []protocol.FileInfo) {
	t := db.newReadWriteTransaction()
	defer t.close()

	var keyBuf []byte
	blockBuf := make([]byte, 4)
	for _, f := range fs {
		if !f.IsDirectory() && !f.IsDeleted() && !f.IsInvalid() {
			name := []byte(f.Name)
			for i, block := range f.Blocks {
				binary.BigEndian.PutUint32(blockBuf, uint32(i))
				keyBuf = t.keyer.GenerateBlockMapKey(keyBuf, folder, block.Hash, name)
				t.Put(keyBuf, blockBuf)
			}
		}
	}
}

func discardFromBlockMap(db *instance, folder []byte, fs []protocol.FileInfo) {
	t := db.newReadWriteTransaction()
	defer t.close()

	var keyBuf []byte
	for _, ef := range fs {
		if !ef.IsDirectory() && !ef.IsDeleted() && !ef.IsInvalid() {
			name := []byte(ef.Name)
			for _, block := range ef.Blocks {
				keyBuf = t.keyer.GenerateBlockMapKey(keyBuf, folder, block.Hash, name)
				t.Delete(keyBuf)
			}
		}
	}
}

func TestBlockMapAddUpdateWipe(t *testing.T) {
	db, f := setup()

	if !dbEmpty(db) {
		t.Fatal("db not empty")
	}

	folder := []byte("folder1")

	f3.Type = protocol.FileInfoTypeDirectory

	addToBlockMap(db, folder, []protocol.FileInfo{f1, f2, f3})

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

	discardFromBlockMap(db, folder, []protocol.FileInfo{f1, f2, f3})

	f1.Deleted = true
	f2.LocalFlags = protocol.FlagLocalMustRescan // one of the invalid markers

	addToBlockMap(db, folder, []protocol.FileInfo{f1, f2, f3})

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

	db.dropFolder(folder)

	if !dbEmpty(db) {
		t.Fatal("db not empty")
	}

	// Should not add
	addToBlockMap(db, folder, []protocol.FileInfo{f1, f2})

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

	folder1 := []byte("folder1")
	folder2 := []byte("folder2")

	addToBlockMap(db, folder1, []protocol.FileInfo{f1})
	addToBlockMap(db, folder2, []protocol.FileInfo{f1})

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

	discardFromBlockMap(db, folder1, []protocol.FileInfo{f1})

	f1.Deleted = true

	addToBlockMap(db, folder1, []protocol.FileInfo{f1})

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

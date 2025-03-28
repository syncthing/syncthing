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
)

var (
	f1, f2, f3 protocol.FileInfo
	folders    = []string{"folder1", "folder2"}
)

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

func setup(t testing.TB) (*Lowlevel, *BlockFinder) {
	t.Helper()
	db := newLowlevelMemory(t)
	return db, NewBlockFinder(db)
}

func dbEmpty(db *Lowlevel) bool {
	iter, err := db.NewPrefixIterator([]byte{KeyTypeBlock})
	if err != nil {
		panic(err)
	}
	defer iter.Release()
	return !iter.Next()
}

func addToBlockMap(db *Lowlevel, folder []byte, fs []protocol.FileInfo) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	var keyBuf []byte
	blockBuf := make([]byte, 4)
	for _, f := range fs {
		if !f.IsDirectory() && !f.IsDeleted() && !f.IsInvalid() {
			name := []byte(f.Name)
			for i, block := range f.Blocks {
				binary.BigEndian.PutUint32(blockBuf, uint32(i))
				keyBuf, err = t.keyer.GenerateBlockMapKey(keyBuf, folder, block.Hash, name)
				if err != nil {
					return err
				}
				if err := t.Put(keyBuf, blockBuf); err != nil {
					return err
				}
			}
		}
	}
	return t.Commit()
}

func discardFromBlockMap(db *Lowlevel, folder []byte, fs []protocol.FileInfo) error {
	t, err := db.newReadWriteTransaction()
	if err != nil {
		return err
	}
	defer t.close()

	var keyBuf []byte
	for _, ef := range fs {
		if !ef.IsDirectory() && !ef.IsDeleted() && !ef.IsInvalid() {
			name := []byte(ef.Name)
			for _, block := range ef.Blocks {
				keyBuf, err = t.keyer.GenerateBlockMapKey(keyBuf, folder, block.Hash, name)
				if err != nil {
					return err
				}
				if err := t.Delete(keyBuf); err != nil {
					return err
				}
			}
		}
	}
	return t.Commit()
}

func TestBlockMapAddUpdateWipe(t *testing.T) {
	db, f := setup(t)
	defer db.Close()

	if !dbEmpty(db) {
		t.Fatal("db not empty")
	}

	folder := []byte("folder1")

	f3.Type = protocol.FileInfoTypeDirectory

	if err := addToBlockMap(db, folder, []protocol.FileInfo{f1, f2, f3}); err != nil {
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

	if err := discardFromBlockMap(db, folder, []protocol.FileInfo{f1, f2, f3}); err != nil {
		t.Fatal(err)
	}

	f1.Deleted = true
	f2.LocalFlags = protocol.FlagLocalMustRescan // one of the invalid markers

	if err := addToBlockMap(db, folder, []protocol.FileInfo{f1, f2, f3}); err != nil {
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

	if err := db.dropFolder(folder); err != nil {
		t.Fatal(err)
	}

	if !dbEmpty(db) {
		t.Fatal("db not empty")
	}

	// Should not add
	if err := addToBlockMap(db, folder, []protocol.FileInfo{f1, f2}); err != nil {
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
	db, f := setup(t)
	defer db.Close()

	folder1 := []byte("folder1")
	folder2 := []byte("folder2")

	if err := addToBlockMap(db, folder1, []protocol.FileInfo{f1}); err != nil {
		t.Fatal(err)
	}
	if err := addToBlockMap(db, folder2, []protocol.FileInfo{f1}); err != nil {
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

	if err := discardFromBlockMap(db, folder1, []protocol.FileInfo{f1}); err != nil {
		t.Fatal(err)
	}

	f1.Deleted = true

	if err := addToBlockMap(db, folder1, []protocol.FileInfo{f1}); err != nil {
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

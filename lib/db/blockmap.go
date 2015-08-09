// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// Package db provides a set type to track local/remote files with newness
// checks. We must do a certain amount of normalization in here. We will get
// fed paths with either native or wire-format separators and encodings
// depending on who calls us. We transform paths to wire-format (NFC and
// slashes) on the way to the database, and transform to native format
// (varying separator and encoding) on the way back out.
package db

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/sync"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var blockFinder *BlockFinder

type BlockMap struct {
	db     *leveldb.DB
	folder string
}

func NewBlockMap(db *leveldb.DB, folder string) *BlockMap {
	return &BlockMap{
		db:     db,
		folder: folder,
	}
}

// Add files to the block map, ignoring any deleted or invalid files.
func (m *BlockMap) Add(files []protocol.FileInfo) error {
	batch := new(leveldb.Batch)
	buf := make([]byte, 4)
	for _, file := range files {
		if file.IsDirectory() || file.IsDeleted() || file.IsInvalid() {
			continue
		}

		for i, block := range file.Blocks {
			binary.BigEndian.PutUint32(buf, uint32(i))
			batch.Put(m.blockKey(block.Hash, file.Name), buf)
		}
	}
	return m.db.Write(batch, nil)
}

// Update block map state, removing any deleted or invalid files.
func (m *BlockMap) Update(files []protocol.FileInfo) error {
	batch := new(leveldb.Batch)
	buf := make([]byte, 4)
	for _, file := range files {
		if file.IsDirectory() {
			continue
		}

		if file.IsDeleted() || file.IsInvalid() {
			for _, block := range file.Blocks {
				batch.Delete(m.blockKey(block.Hash, file.Name))
			}
			continue
		}

		for i, block := range file.Blocks {
			binary.BigEndian.PutUint32(buf, uint32(i))
			batch.Put(m.blockKey(block.Hash, file.Name), buf)
		}
	}
	return m.db.Write(batch, nil)
}

// Discard block map state, removing the given files
func (m *BlockMap) Discard(files []protocol.FileInfo) error {
	batch := new(leveldb.Batch)
	for _, file := range files {
		for _, block := range file.Blocks {
			batch.Delete(m.blockKey(block.Hash, file.Name))
		}
	}
	return m.db.Write(batch, nil)
}

// Drop block map, removing all entries related to this block map from the db.
func (m *BlockMap) Drop() error {
	batch := new(leveldb.Batch)
	iter := m.db.NewIterator(util.BytesPrefix(m.blockKey(nil, "")[:1+64]), nil)
	defer iter.Release()
	for iter.Next() {
		batch.Delete(iter.Key())
	}
	if iter.Error() != nil {
		return iter.Error()
	}
	return m.db.Write(batch, nil)
}

func (m *BlockMap) blockKey(hash []byte, file string) []byte {
	return toBlockKey(hash, m.folder, file)
}

type BlockFinder struct {
	db      *leveldb.DB
	folders []string
	mut     sync.RWMutex
}

func NewBlockFinder(db *leveldb.DB, cfg *config.Wrapper) *BlockFinder {
	if blockFinder != nil {
		return blockFinder
	}

	f := &BlockFinder{
		db:  db,
		mut: sync.NewRWMutex(),
	}

	f.CommitConfiguration(config.Configuration{}, cfg.Raw())
	cfg.Subscribe(f)

	return f
}

// VerifyConfiguration implementes the config.Committer interface
func (f *BlockFinder) VerifyConfiguration(from, to config.Configuration) error {
	return nil
}

// CommitConfiguration implementes the config.Committer interface
func (f *BlockFinder) CommitConfiguration(from, to config.Configuration) bool {
	folders := make([]string, len(to.Folders))
	for i, folder := range to.Folders {
		folders[i] = folder.ID
	}

	sort.Strings(folders)

	f.mut.Lock()
	f.folders = folders
	f.mut.Unlock()

	return true
}

func (f *BlockFinder) String() string {
	return fmt.Sprintf("BlockFinder@%p", f)
}

// Iterate takes an iterator function which iterates over all matching blocks
// for the given hash. The iterator function has to return either true (if
// they are happy with the block) or false to continue iterating for whatever
// reason. The iterator finally returns the result, whether or not a
// satisfying block was eventually found.
func (f *BlockFinder) Iterate(hash []byte, iterFn func(string, string, int32) bool) bool {
	f.mut.RLock()
	folders := f.folders
	f.mut.RUnlock()
	for _, folder := range folders {
		key := toBlockKey(hash, folder, "")
		iter := f.db.NewIterator(util.BytesPrefix(key), nil)
		defer iter.Release()

		for iter.Next() && iter.Error() == nil {
			folder, file := fromBlockKey(iter.Key())
			index := int32(binary.BigEndian.Uint32(iter.Value()))
			if iterFn(folder, osutil.NativeFilename(file), index) {
				return true
			}
		}
	}
	return false
}

// Fix repairs incorrect blockmap entries, removing the old entry and
// replacing it with a new entry for the given block
func (f *BlockFinder) Fix(folder, file string, index int32, oldHash, newHash []byte) error {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(index))

	batch := new(leveldb.Batch)
	batch.Delete(toBlockKey(oldHash, folder, file))
	batch.Put(toBlockKey(newHash, folder, file), buf)
	return f.db.Write(batch, nil)
}

// m.blockKey returns a byte slice encoding the following information:
//	   keyTypeBlock (1 byte)
//	   folder (64 bytes)
//	   block hash (32 bytes)
//	   file name (variable size)
func toBlockKey(hash []byte, folder, file string) []byte {
	o := make([]byte, 1+64+32+len(file))
	o[0] = KeyTypeBlock
	copy(o[1:], []byte(folder))
	copy(o[1+64:], []byte(hash))
	copy(o[1+64+32:], []byte(file))
	return o
}

func fromBlockKey(data []byte) (string, string) {
	if len(data) < 1+64+32+1 {
		panic("Incorrect key length")
	}
	if data[0] != KeyTypeBlock {
		panic("Incorrect key type")
	}

	file := string(data[1+64+32:])

	slice := data[1 : 1+64]
	izero := bytes.IndexByte(slice, 0)
	if izero > -1 {
		return string(slice[:izero]), file
	}
	return string(slice), file
}

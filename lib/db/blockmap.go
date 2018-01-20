// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"encoding/binary"
	"fmt"

	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var blockFinder *BlockFinder

const maxBatchSize = 1000

type BlockMap struct {
	db       *Instance
	folder   []byte
	folderID uint32
}

func NewBlockMap(db *Instance, folder []byte) *BlockMap {
	return &BlockMap{
		db:       db,
		folder:   folder,
		folderID: db.folderIdx.ID(folder),
	}
}

// Update block map state, i.e. remove the old file first and add or remove
// (when deleted or invalid) the new file.
func (m *BlockMap) Update(files []protocol.FileInfo) error {
	batch := new(leveldb.Batch)
	buf := make([]byte, 4)
	var key []byte
	for _, file := range files {
		if ef, ok := m.db.getFile([]byte(m.folder), protocol.LocalDeviceID[:], []byte(file.Name)); ok {
			if ef.Version.Equal(file.Version) && ef.Invalid == file.Invalid {
				continue
			}
			for len(ef.Blocks) != 0 {
				if err := m.checkBatchLen(batch); err != nil {
					return err
				}
				key = m.blockKeyInto(key, ef.Blocks[0].Hash, ef.Name)
				batch.Delete(key)
				// Release memory earlier
				ef.Blocks = ef.Blocks[1:]
			}
		}

		if file.IsDirectory() {
			continue
		}

		if file.IsDeleted() || file.IsInvalid() {
			for _, block := range file.Blocks {
				if err := m.checkBatchLen(batch); err != nil {
					return err
				}
				key = m.blockKeyInto(key, block.Hash, file.Name)
				batch.Delete(key)
			}
			continue
		}

		for i, block := range file.Blocks {
			if err := m.checkBatchLen(batch); err != nil {
				return err
			}
			binary.BigEndian.PutUint32(buf, uint32(i))
			key = m.blockKeyInto(key, block.Hash, file.Name)
			batch.Put(key, buf)
		}
	}
	return m.db.Write(batch, nil)
}

// Drop block map, removing all entries related to this block map from the db.
func (m *BlockMap) Drop() error {
	batch := new(leveldb.Batch)
	iter := m.db.NewIterator(util.BytesPrefix(m.blockKeyInto(nil, nil, "")[:keyPrefixLen+keyFolderLen]), nil)
	defer iter.Release()
	for iter.Next() {
		if err := m.checkBatchLen(batch); err != nil {
			return err
		}

		batch.Delete(iter.Key())
	}
	if iter.Error() != nil {
		return iter.Error()
	}
	return m.db.Write(batch, nil)
}

func (m *BlockMap) blockKeyInto(o, hash []byte, file string) []byte {
	return blockKeyInto(o, hash, m.folderID, file)
}

func (m *BlockMap) checkBatchLen(batch *leveldb.Batch) error {
	if batch.Len() < maxBatchSize {
		return nil
	}
	if err := m.db.Write(batch, nil); err != nil {
		return err
	}
	batch.Reset()
	return nil
}

type BlockFinder struct {
	db *Instance
}

func NewBlockFinder(db *Instance) *BlockFinder {
	if blockFinder != nil {
		return blockFinder
	}

	f := &BlockFinder{
		db: db,
	}

	return f
}

func (f *BlockFinder) String() string {
	return fmt.Sprintf("BlockFinder@%p", f)
}

// Iterate takes an iterator function which iterates over all matching blocks
// for the given hash. The iterator function has to return either true (if
// they are happy with the block) or false to continue iterating for whatever
// reason. The iterator finally returns the result, whether or not a
// satisfying block was eventually found.
func (f *BlockFinder) Iterate(folders []string, hash []byte, iterFn func(string, string, int32) bool) bool {
	var key []byte
	for _, folder := range folders {
		folderID := f.db.folderIdx.ID([]byte(folder))
		key = blockKeyInto(key, hash, folderID, "")
		iter := f.db.NewIterator(util.BytesPrefix(key), nil)
		defer iter.Release()

		for iter.Next() && iter.Error() == nil {
			file := blockKeyName(iter.Key())
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

	folderID := f.db.folderIdx.ID([]byte(folder))
	batch := new(leveldb.Batch)
	batch.Delete(blockKeyInto(nil, oldHash, folderID, file))
	batch.Put(blockKeyInto(nil, newHash, folderID, file), buf)
	return f.db.Write(batch, nil)
}

// m.blockKey returns a byte slice encoding the following information:
//	   keyTypeBlock (1 byte)
//	   folder (4 bytes)
//	   block hash (32 bytes)
//	   file name (variable size)
func blockKeyInto(o, hash []byte, folder uint32, file string) []byte {
	reqLen := keyPrefixLen + keyFolderLen + keyHashLen + len(file)
	if cap(o) < reqLen {
		o = make([]byte, reqLen)
	} else {
		o = o[:reqLen]
	}
	o[0] = KeyTypeBlock
	binary.BigEndian.PutUint32(o[keyPrefixLen:], folder)
	copy(o[keyPrefixLen+keyFolderLen:], hash)
	copy(o[keyPrefixLen+keyFolderLen+keyHashLen:], []byte(file))
	return o
}

// blockKeyName returns the file name from the block key
func blockKeyName(data []byte) string {
	if len(data) < keyPrefixLen+keyFolderLen+keyHashLen+1 {
		panic("Incorrect key length")
	}
	if data[0] != KeyTypeBlock {
		panic("Incorrect key type")
	}

	file := string(data[keyPrefixLen+keyFolderLen+keyHashLen:])
	return file
}

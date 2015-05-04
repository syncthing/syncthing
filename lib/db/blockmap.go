// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import (
	"github.com/boltdb/bolt"
	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/lib/osutil"
)

type BlockMap struct {
	db     *BoltDB
	folder []byte
}

var blocksBucketID = []byte("blocks")

const keyBytes = 3

func NewBlockMap(db *BoltDB, folder string) *BlockMap {
	db.Update(func(tx *bolt.Tx) error {
		bkt, err := tx.CreateBucketIfNotExists(blocksBucketID)
		if err != nil {
			panic(err)
		}
		_, err = bkt.CreateBucketIfNotExists([]byte(folder))
		if err != nil {
			panic(err)
		}
		return nil
	})
	return &BlockMap{
		db:     db,
		folder: []byte(folder),
	}
}

// Add files to the block map, ignoring any deleted or invalid files.
func (m *BlockMap) Add(files []protocol.FileInfo) {
	m.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(blocksBucketID).Bucket(m.folder)
		for _, file := range files {
			if file.IsDirectory() || file.IsDeleted() || file.IsInvalid() {
				continue
			}

			for i, block := range file.Blocks {
				key := block.Hash[:keyBytes]
				val := bkt.Get(key)

				var list bmList
				if val != nil {
					if err := list.UnmarshalXDR(val); err != nil {
						panic(err)
					}
				}

				list.entries = append(list.entries, bmEntry{
					name:  file.Name,
					index: int32(i),
				})

				if err := bkt.Put(key, list.MustMarshalXDR()); err != nil {
					panic(err)
				}
			}
		}
		return nil
	})
}

// Update block map state, removing any deleted or invalid files.
func (m *BlockMap) Update(files []protocol.FileInfo) {
	adds := make([]protocol.FileInfo, 0, len(files)/2)
	discards := make([]protocol.FileInfo, 0, len(files)/2)
	for _, file := range files {
		if file.IsDeleted() || file.IsInvalid() {
			discards = append(discards, file)
		} else {
			adds = append(adds, file)
		}
	}
	m.Discard(discards)
	m.Add(adds)
}

// Discard block map state, removing the given files
func (m *BlockMap) Discard(files []protocol.FileInfo) error {
	m.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(blocksBucketID).Bucket(m.folder)
		for _, file := range files {
		nextBlock:
			for i, block := range file.Blocks {
				key := block.Hash[:keyBytes]
				val := bkt.Get(key)
				if val == nil {
					continue
				}

				var list bmList
				if err := list.UnmarshalXDR(val); err != nil {
					panic(err)
				}

				for j, entry := range list.entries {
					if entry.index == int32(i) && entry.name == file.Name {
						list.entries = append(list.entries[:j], list.entries[j+1:]...)
						if err := bkt.Put(key, list.MustMarshalXDR()); err != nil {
							panic(err)
						}
						continue nextBlock
					}
				}
			}
		}
		return nil
	})
	return nil
}

// Drop block map, removing all entries related to this block map from the db.
func (m *BlockMap) Drop() error {
	m.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(blocksBucketID)
		if err := bkt.DeleteBucket(m.folder); err != nil {
			panic(err)
		}
		if _, err := bkt.CreateBucketIfNotExists(m.folder); err != nil {
			panic(err)
		}
		return nil
	})
	return nil
}

// Iterate takes an iterator function which iterates over all matching blocks
// for the given hash. The iterator function has to return either true (if
// they are happy with the block) or false to continue iterating for whatever
// reason. The iterator finally returns the result, whether or not a
// satisfying block was eventually found.
func (m *BlockMap) Iterate(hash []byte, iterFn func(file string, index int) bool) bool {
	key := hash[:keyBytes]
	var val []byte
	m.db.View(func(tx *bolt.Tx) error {
		val = tx.Bucket(blocksBucketID).Bucket(m.folder).Get(key)
		return nil
	})
	if val != nil {
		var list bmList
		if err := list.UnmarshalXDR(val); err != nil {
			panic(err)
		}
		for _, entry := range list.entries {
			if iterFn(osutil.NativeFilename(entry.name), int(entry.index)) {
				return true
			}
		}
	}

	return false
}

// Fix repairs incorrect blockmap entries, removing the old entry and
// replacing it with a new entry for the given block
func (m *BlockMap) Fix(file string, index int, oldHash, newHash []byte) {
	m.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(blocksBucketID).Bucket(m.folder)

		key := oldHash[:keyBytes]
		val := bkt.Get(key)
		if val != nil {
			var list bmList
			if err := list.UnmarshalXDR(val); err != nil {
				panic(err)
			}

			for j, entry := range list.entries {
				if entry.index == int32(index) && entry.name == file {
					list.entries = append(list.entries[:j], list.entries[j+1:]...)
					if err := bkt.Put(key, list.MustMarshalXDR()); err != nil {
						panic(err)
					}
					break
				}
			}
		}

		var list bmList
		key = newHash[:keyBytes]
		val = bkt.Get(key)
		if val != nil {
			if err := list.UnmarshalXDR(val); err != nil {
				panic(err)
			}
		}

		list.entries = append(list.entries, bmEntry{
			name:  file,
			index: int32(index),
		})
		if err := bkt.Put(key, list.MustMarshalXDR()); err != nil {
			panic(err)
		}

		return nil
	})
}

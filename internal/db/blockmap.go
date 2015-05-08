// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import (
	"github.com/Workiva/go-datastructures/trie/ctrie"
	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/internal/osutil"
)

type BlockMap struct {
	trie *ctrie.Ctrie
}

func NewBlockMap() *BlockMap {
	return &BlockMap{
		trie: ctrie.New(nil),
	}
}

type bmEntry struct {
	name  string
	index int
}

// Add files to the block map, ignoring any deleted or invalid files.
func (m *BlockMap) Add(files []protocol.FileInfo) {
	for _, file := range files {
		if file.IsDirectory() || file.IsDeleted() || file.IsInvalid() {
			continue
		}

	nextBlock:
		for i, block := range file.Blocks {
			val, ok := m.trie.Lookup(block.Hash)
			if !ok {
				// New block, add it
				m.trie.Insert(block.Hash, []bmEntry{{
					name:  file.Name,
					index: i,
				}})
				continue
			}

			// Existing block, add to list, if it's not already there.
			entries := val.([]bmEntry)
			for _, e := range entries {
				if e.index == i && e.name == file.Name {
					// Block is already in the registry
					continue nextBlock
				}
			}

			entries = append(entries, bmEntry{
				name:  file.Name,
				index: i,
			})

			m.trie.Insert(block.Hash, entries)
		}
	}
}

// Update block map state, removing any deleted or invalid files.
func (m *BlockMap) Update(files []protocol.FileInfo) {
	for i, file := range files {
		if file.IsDeleted() || file.IsInvalid() {
			m.Discard(files[i : i+1])
		} else {
			m.Add(files[i : i+1])
		}
	}
}

// Discard block map state, removing the given files
func (m *BlockMap) Discard(files []protocol.FileInfo) error {
	for _, file := range files {
	nextBlock:
		for i, block := range file.Blocks {
			val, ok := m.trie.Lookup(block.Hash)
			if !ok {
				continue nextBlock
			}
			entries := val.([]bmEntry)
			if len(entries) == 1 {
				m.trie.Remove(block.Hash)
				continue nextBlock
			}
			for j, entry := range entries {
				if entry.index == i && entry.name == file.Name {
					entries = append(entries[:j], entries[j+1:]...)
					m.trie.Insert(block.Hash, entries)
					continue nextBlock
				}
			}
		}
	}
	return nil
}

// Drop block map, removing all entries related to this block map from the db.
func (m *BlockMap) Drop() error {
	m.trie = ctrie.New(nil)
	return nil
}

// Iterate takes an iterator function which iterates over all matching blocks
// for the given hash. The iterator function has to return either true (if
// they are happy with the block) or false to continue iterating for whatever
// reason. The iterator finally returns the result, whether or not a
// satisfying block was eventually found.
func (m *BlockMap) Iterate(hash []byte, iterFn func(file string, index int) bool) bool {
	val, ok := m.trie.Lookup(hash)
	if !ok {
		return false
	}
	for _, entry := range val.([]bmEntry) {
		if iterFn(osutil.NativeFilename(entry.name), entry.index) {
			return true
		}
	}
	return false
}

// Fix repairs incorrect blockmap entries, removing the old entry and
// replacing it with a new entry for the given block
func (m *BlockMap) Fix(folder, file string, index int, oldHash, newHash []byte) {
	/*buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(index))

	batch := new(leveldb.Batch)
	batch.Delete(toBlockKey(oldHash, folder, file))
	batch.Put(toBlockKey(newHash, folder, file), buf)
	return f.db.Write(batch, nil)*/
}

// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import (
	"encoding/binary"

	"github.com/syncthing/protocol"
	"github.com/syncthing/syncthing/internal/osutil"
)

const (
	keyBits = 20
	keyMask = 1<<keyBits - 1
	idxSize = 1 << keyBits
)

type BlockMap struct {
	sc  *stringCache
	idx map[int32][]bmEntry
}

type bmEntry struct {
	name  int32
	index int32
}

func NewBlockMap() *BlockMap {
	return &BlockMap{
		sc:  newStringCache(),
		idx: make(map[int32][]bmEntry),
	}
}

func keyOf(hash []byte) int32 {
	return int32(binary.BigEndian.Uint32(hash) & keyMask)
}

// Add files to the block map, ignoring any deleted or invalid files.
func (m *BlockMap) Add(files []protocol.FileInfo) {
	for _, file := range files {
		if file.IsDirectory() || file.IsDeleted() || file.IsInvalid() {
			continue
		}
		idx := m.sc.Index(file.Name)

	nextBlock:
		for i, block := range file.Blocks {
			key := keyOf(block.Hash)
			entries := m.idx[key]
			if entries == nil {
				// New block, add it
				m.idx[key] = []bmEntry{{
					name:  idx,
					index: int32(i),
				}}
				continue
			}

			// Existing block, add to list, if it's not already there.
			for _, e := range entries {
				if e.index == int32(i) && e.name == idx {
					// Block is already in the registry
					continue nextBlock
				}
			}

			entries = append(entries, bmEntry{
				name:  idx,
				index: int32(i),
			})

			m.idx[key] = entries
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
			key := keyOf(block.Hash)
			entries := m.idx[key]
			if entries == nil {
				continue nextBlock
			}
			if len(entries) == 1 {
				m.idx[key] = nil
				continue nextBlock
			}

			idx := m.sc.Index(file.Name)
			for j, entry := range entries {
				if entry.index == int32(i) && entry.name == idx {
					entries = append(entries[:j], entries[j+1:]...)
					m.idx[key] = entries
					continue nextBlock
				}
			}
		}
	}
	return nil
}

// Drop block map, removing all entries related to this block map from the db.
func (m *BlockMap) Drop() error {
	m.idx = make(map[int32][]bmEntry)
	return nil
}

// Iterate takes an iterator function which iterates over all matching blocks
// for the given hash. The iterator function has to return either true (if
// they are happy with the block) or false to continue iterating for whatever
// reason. The iterator finally returns the result, whether or not a
// satisfying block was eventually found.
func (m *BlockMap) Iterate(hash []byte, iterFn func(file string, index int) bool) bool {
	key := keyOf(hash)
	entries := m.idx[key]
	for _, entry := range entries {
		if iterFn(osutil.NativeFilename(m.sc.Lookup(entry.name)), int(entry.index)) {
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

func (m *BlockMap) Stats() (maxLen int, avgLen, fill float64) {
	max := 0
	tot := 0
	cnt := 0
	for _, l := range m.idx {
		if l != nil {
			cnt++
			tot += len(l)
			if len(l) > max {
				max = len(l)
			}
		}
	}
	return max, float64(tot) / float64(cnt), float64(cnt) / idxSize
}

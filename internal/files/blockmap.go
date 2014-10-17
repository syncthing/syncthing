// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

// Package files provides a set type to track local/remote files with newness
// checks. We must do a certain amount of normalization in here. We will get
// fed paths with either native or wire-format separators and encodings
// depending on who calls us. We transform paths to wire-format (NFC and
// slashes) on the way to the database, and transform to native format
// (varying separator and encoding) on the way back out.

package files

import (
	"bytes"
	"encoding/binary"
	"sort"
	"sync"

	"github.com/syncthing/syncthing/internal/config"
	"github.com/syncthing/syncthing/internal/protocol"

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

func NewBlockFinder(db *leveldb.DB, cfg *config.ConfigWrapper) *BlockFinder {
	if blockFinder != nil {
		return blockFinder
	}

	f := &BlockFinder{
		db: db,
	}
	f.Changed(cfg.Raw())
	cfg.Subscribe(f)
	return f
}

// Implements config.Handler interface
func (f *BlockFinder) Changed(cfg config.Configuration) error {
	folders := make([]string, len(cfg.Folders))
	for i, folder := range cfg.Folders {
		folders[i] = folder.ID
	}

	sort.Strings(folders)

	f.mut.Lock()
	f.folders = folders
	f.mut.Unlock()

	return nil
}

// An iterator function which iterates over all matching blocks for the given
// hash. The iterator function has to return either true (if they are happy with
// the block) or false to continue iterating for whatever reason.
// The iterator finally returns the result, whether or not a satisfying block
// was eventually found.
func (f *BlockFinder) Iterate(hash []byte, iterFn func(string, string, uint32) bool) bool {
	f.mut.RLock()
	folders := f.folders
	f.mut.RUnlock()
	for _, folder := range folders {
		key := toBlockKey(hash, folder, "")
		iter := f.db.NewIterator(util.BytesPrefix(key), nil)
		defer iter.Release()

		for iter.Next() && iter.Error() == nil {
			folder, file := fromBlockKey(iter.Key())
			index := binary.BigEndian.Uint32(iter.Value())
			if iterFn(folder, nativeFilename(file), index) {
				return true
			}
		}
	}
	return false
}

// m.blockKey returns a byte slice encoding the following information:
//	   keyTypeBlock (1 byte)
//	   folder (64 bytes)
//	   block hash (32 bytes)
//	   file name (variable size)
func toBlockKey(hash []byte, folder, file string) []byte {
	o := make([]byte, 1+64+32+len(file))
	o[0] = keyTypeBlock
	copy(o[1:], []byte(folder))
	copy(o[1+64:], []byte(hash))
	copy(o[1+64+32:], []byte(file))
	return o
}

func fromBlockKey(data []byte) (string, string) {
	if len(data) < 1+64+32+1 {
		panic("Incorrect key length")
	}
	if data[0] != keyTypeBlock {
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

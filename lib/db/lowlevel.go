// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"encoding/binary"
	"time"

	"github.com/syncthing/syncthing/lib/db/backend"
)

const (
	// We set the bloom filter capacity to handle 100k individual items with
	// a false positive probability of 1% for the first pass. Once we know
	// how many items we have we will use that number instead, if it's more
	// than 100k. For fewer than 100k items we will just get better false
	// positive rate instead.
	indirectGCBloomCapacity          = 100000
	indirectGCBloomFalsePositiveRate = 0.01     // 1%
	indirectGCBloomMaxBytes          = 32 << 20 // Use at most 32MiB memory, which covers our desired FP rate at 27 M items
	indirectGCDefaultInterval        = 13 * time.Hour
	indirectGCTimeKey                = "lastIndirectGCTime"

	// Use indirection for the block list when it exceeds this many entries
	blocksIndirectionCutoff = 3
	// Use indirection for the version vector when it exceeds this many entries
	versionIndirectionCutoff = 10

	recheckDefaultInterval = 300 * 24 * time.Hour

	needsRepairSuffix = ".needsrepair"
)

// deprecatedLowlevel is the lowest level database interface. It has a very simple
// purpose: hold the actual backend database, and the in-memory state
// that belong to that database. In the same way that a single on disk
// database can only be opened once, there should be only one deprecatedLowlevel for
// any given backend.
type deprecatedLowlevel struct {
	backend.Backend
	folderIdx *smallIndex
	deviceIdx *smallIndex
	keyer     keyer
}

func NewLowlevel(backend backend.Backend) (*deprecatedLowlevel, error) {
	// Only log restarts in debug mode.
	db := &deprecatedLowlevel{
		Backend:   backend,
		folderIdx: newSmallIndex(backend, []byte{KeyTypeFolderIdx}),
		deviceIdx: newSmallIndex(backend, []byte{KeyTypeDeviceIdx}),
	}
	db.keyer = newDefaultKeyer(db.folderIdx, db.deviceIdx)
	return db, nil
}

// ListFolders returns the list of folders currently in the database
func (db *deprecatedLowlevel) ListFolders() []string {
	return db.folderIdx.Values()
}

func (db *deprecatedLowlevel) IterateMtimes(fn func(folder, name string, ondisk, virtual time.Time) error) error {
	it, err := db.NewPrefixIterator([]byte{KeyTypeVirtualMtime})
	if err != nil {
		return err
	}
	defer it.Release()
	for it.Next() {
		key := it.Key()[1:]
		folderID, ok := db.folderIdx.Val(binary.BigEndian.Uint32(key))
		if !ok {
			continue
		}
		name := key[4:]
		val := it.Value()
		var ondisk, virtual time.Time
		if err := ondisk.UnmarshalBinary(val[:len(val)/2]); err != nil {
			continue
		}
		if err := virtual.UnmarshalBinary(val[len(val)/2:]); err != nil {
			continue
		}
		if err := fn(string(folderID), string(name), ondisk, virtual); err != nil {
			return err
		}
	}
	return it.Error()
}

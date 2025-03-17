// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package olddb

import (
	"encoding/binary"
	"time"

	"github.com/syncthing/syncthing/internal/db/olddb/backend"
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

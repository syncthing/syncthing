// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"github.com/syncthing/syncthing/lib/db/backend"
)

// Lowlevel is the lowest level database interface. It has a very simple
// purpose: hold the actual backend database, and the in-memory state
// that belong to that database. In the same way that a single on disk
// database can only be opened once, there should be only one Lowlevel for
// any given backend.
type Lowlevel struct {
	backend.Backend
	folderIdx *smallIndex
	deviceIdx *smallIndex
}

// NewLowlevel wraps the given *leveldb.DB into a *lowlevel
func NewLowlevel(db backend.Backend) *Lowlevel {
	return &Lowlevel{
		Backend:   db,
		folderIdx: newSmallIndex(db, []byte{KeyTypeFolderIdx}),
		deviceIdx: newSmallIndex(db, []byte{KeyTypeDeviceIdx}),
	}
}

// ListFolders returns the list of folders currently in the database
func (db *Lowlevel) ListFolders() []string {
	return db.folderIdx.Values()
}

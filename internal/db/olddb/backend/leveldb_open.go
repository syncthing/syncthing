// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package backend

import (
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

const dbMaxOpenFiles = 100

// OpenLevelDBRO attempts to open the database at the given location, read
// only.
func OpenLevelDBRO(location string) (Backend, error) {
	opts := &opt.Options{
		OpenFilesCacheCapacity: dbMaxOpenFiles,
		ReadOnly:               true,
	}
	ldb, err := open(location, opts)
	if err != nil {
		return nil, err
	}
	return newLeveldbBackend(ldb, location), nil
}

func open(location string, opts *opt.Options) (*leveldb.DB, error) {
	return leveldb.OpenFile(location, opts)
}

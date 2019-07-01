// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"encoding/json"
	"io"
	"os"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// writeJSONS serializes the database to a JSON stream that can be checked
// in to the repo and used for tests.
func writeJSONS(w io.Writer, db *leveldb.DB) {
	it := db.NewIterator(&util.Range{}, nil)
	defer it.Release()
	enc := json.NewEncoder(w)
	for it.Next() {
		enc.Encode(map[string][]byte{
			"k": it.Key(),
			"v": it.Value(),
		})
	}
}

// we know this function isn't generally used, nonetheless we want it in
// here and the linter to not complain.
var _ = writeJSONS

// openJSONS reads a JSON stream file into a leveldb.DB
func openJSONS(file string) (*leveldb.DB, error) {
	fd, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(fd)

	db, _ := leveldb.Open(storage.NewMemStorage(), nil)

	for {
		var row map[string][]byte

		err := dec.Decode(&row)
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		db.Put(row["k"], row["v"], nil)
	}

	return db, nil
}

// The following commented tests were used to generate jsons files to stdout for
// future tests and are kept here for reference (reuse).

// TestGenerateIgnoredFilesDB generates a database with files with invalid flags,
// local and remote, in the format used in 0.14.48.
// func TestGenerateIgnoredFilesDB(t *testing.T) {
// 	db := OpenMemory()
// 	fs := NewFileSet("test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), db)
// 	fs.Update(protocol.LocalDeviceID, []protocol.FileInfo{
// 		{ // invalid (ignored) file
// 			Name:    "foo",
// 			Type:    protocol.FileInfoTypeFile,
// 			Invalid: true,
// 			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 1, Value: 1000}}},
// 		},
// 		{ // regular file
// 			Name:    "bar",
// 			Type:    protocol.FileInfoTypeFile,
// 			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 1, Value: 1001}}},
// 		},
// 	})
// 	fs.Update(protocol.DeviceID{42}, []protocol.FileInfo{
// 		{ // invalid file
// 			Name:    "baz",
// 			Type:    protocol.FileInfoTypeFile,
// 			Invalid: true,
// 			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: 1000}}},
// 		},
// 		{ // regular file
// 			Name:    "quux",
// 			Type:    protocol.FileInfoTypeFile,
// 			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: 1002}}},
// 		},
// 	})
// 	writeJSONS(os.Stdout, db.DB)
// }

// TestGenerateUpdate0to3DB generates a database with files with invalid flags, prefixed
// by a slash and other files to test database migration from version 0 to 3, in the
// format used in 0.14.45.
// func TestGenerateUpdate0to3DB(t *testing.T) {
// 	db := OpenMemory()
// 	fs := NewFileSet(update0to3Folder, fs.NewFilesystem(fs.FilesystemTypeBasic, "."), db)
// 	for devID, files := range haveUpdate0to3 {
// 		fs.Update(devID, files)
// 	}
// 	writeJSONS(os.Stdout, db.DB)
// }

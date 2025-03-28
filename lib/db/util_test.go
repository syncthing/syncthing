// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
)

// writeJSONS serializes the database to a JSON stream that can be checked
// in to the repo and used for tests.
func writeJSONS(w io.Writer, db backend.Backend) {
	it, err := db.NewPrefixIterator(nil)
	if err != nil {
		panic(err)
	}
	defer it.Release()
	enc := json.NewEncoder(w)
	for it.Next() {
		err := enc.Encode(map[string][]byte{
			"k": it.Key(),
			"v": it.Value(),
		})
		if err != nil {
			panic(err)
		}
	}
}

// we know this function isn't generally used, nonetheless we want it in
// here and the linter to not complain.
var _ = writeJSONS

// openJSONS reads a JSON stream file into a backend DB
func openJSONS(file string) (backend.Backend, error) {
	fd, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(fd)

	db := backend.OpenMemory()

	for {
		var row map[string][]byte

		err := dec.Decode(&row)
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		if err := db.Put(row["k"], row["v"]); err != nil {
			return nil, err
		}
	}

	return db, nil
}

func newLowlevel(t testing.TB, backend backend.Backend) *Lowlevel {
	t.Helper()
	ll, err := NewLowlevel(backend, events.NoopLogger)
	if err != nil {
		t.Fatal(err)
	}
	return ll
}

func newLowlevelMemory(t testing.TB) *Lowlevel {
	return newLowlevel(t, backend.OpenMemory())
}

func newFileSet(t testing.TB, folder string, db *Lowlevel) *FileSet {
	t.Helper()
	fset, err := NewFileSet(folder, db)
	if err != nil {
		t.Fatal(err)
	}
	return fset
}

func snapshot(t testing.TB, fset *FileSet) *Snapshot {
	t.Helper()
	snap, err := fset.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// The following commented tests were used to generate jsons files to stdout for
// future tests and are kept here for reference (reuse).

// TestGenerateIgnoredFilesDB generates a database with files with invalid flags,
// local and remote, in the format used in 0.14.48.
// func TestGenerateIgnoredFilesDB(t *testing.T) {
// 	db := OpenMemory()
// 	fs := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), db)
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
// 	fs := newFileSet(t, update0to3Folder, fs.NewFilesystem(fs.FilesystemTypeBasic, "."), db)
// 	for devID, files := range haveUpdate0to3 {
// 		fs.Update(devID, files)
// 	}
// 	writeJSONS(os.Stdout, db.DB)
// }

// func TestGenerateUpdateTo10(t *testing.T) {
// 	db := newLowlevelMemory(t)
// 	defer db.Close()

// 	if err := UpdateSchema(db); err != nil {
// 		t.Fatal(err)
// 	}

// 	fs := newFileSet(t, "test", fs.NewFilesystem(fs.FilesystemTypeFake, ""), db)

// 	files := []protocol.FileInfo{
// 		{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Deleted: true, Sequence: 1},
// 		{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(2), Sequence: 2},
// 		{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Deleted: true, Sequence: 3},
// 	}
// 	fs.Update(protocol.LocalDeviceID, files)
// 	files[1].Version = files[1].Version.Update(remoteDevice0.Short())
// 	files[1].Deleted = true
// 	files[2].Version = files[2].Version.Update(remoteDevice0.Short())
// 	files[2].Blocks = genBlocks(1)
// 	files[2].Deleted = false
// 	fs.Update(remoteDevice0, files)

// 	fd, err := os.Create("./testdata/v1.4.0-updateTo10.json")
// 	if err != nil {
// 		panic(err)
// 	}
// 	defer fd.Close()
// 	writeJSONS(fd, db)
// }

func TestFileInfoBatchError(t *testing.T) {
	// Verify behaviour of the flush function returning an error.

	var errReturn error
	var called int
	b := NewFileInfoBatch(func([]protocol.FileInfo) error {
		called += 1
		return errReturn
	})

	// Flush should work when the flush function error is nil
	b.Append(protocol.FileInfo{Name: "test"})
	if err := b.Flush(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if called != 1 {
		t.Fatalf("expected 1, got %d", called)
	}

	// Flush should fail with an error retur
	errReturn = errors.New("problem")
	b.Append(protocol.FileInfo{Name: "test"})
	if err := b.Flush(); err != errReturn {
		t.Fatalf("expected %v, got %v", errReturn, err)
	}
	if called != 2 {
		t.Fatalf("expected 2, got %d", called)
	}

	// Flush function should not be called again when it's already errored,
	// same error should be returned by Flush()
	if err := b.Flush(); err != errReturn {
		t.Fatalf("expected %v, got %v", errReturn, err)
	}
	if called != 2 {
		t.Fatalf("expected 2, got %d", called)
	}

	// Reset should clear the error (and the file list)
	errReturn = nil
	b.Reset()
	b.Append(protocol.FileInfo{Name: "test"})
	if err := b.Flush(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if called != 3 {
		t.Fatalf("expected 3, got %d", called)
	}
}

// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// Reader is the interface that wraps basic Get and NewIterator methods.
// This interface implemented by both DB and Snapshot.
type Reader interface {
	Get(key []byte, ro *opt.ReadOptions) (value []byte, err error)
	NewIterator(slice *util.Range, ro *opt.ReadOptions) iterator.Iterator
}

type Sizes []uint64

// Sum returns sum of the sizes.
func (p Sizes) Sum() (n uint64) {
	for _, s := range p {
		n += s
	}
	return n
}

// Logging.
func (db *DB) log(v ...interface{})                 { db.s.log(v...) }
func (db *DB) logf(format string, v ...interface{}) { db.s.logf(format, v...) }

// Check and clean files.
func (db *DB) checkAndCleanFiles() error {
	v := db.s.version()
	defer v.release()

	tablesMap := make(map[uint64]bool)
	for _, tables := range v.tables {
		for _, t := range tables {
			tablesMap[t.file.Num()] = false
		}
	}

	files, err := db.s.getFiles(storage.TypeAll)
	if err != nil {
		return err
	}

	var nTables int
	var rem []storage.File
	for _, f := range files {
		keep := true
		switch f.Type() {
		case storage.TypeManifest:
			keep = f.Num() >= db.s.manifestFile.Num()
		case storage.TypeJournal:
			if db.frozenJournalFile != nil {
				keep = f.Num() >= db.frozenJournalFile.Num()
			} else {
				keep = f.Num() >= db.journalFile.Num()
			}
		case storage.TypeTable:
			_, keep = tablesMap[f.Num()]
			if keep {
				tablesMap[f.Num()] = true
				nTables++
			}
		}

		if !keep {
			rem = append(rem, f)
		}
	}

	if nTables != len(tablesMap) {
		var missing []*storage.FileInfo
		for num, present := range tablesMap {
			if !present {
				missing = append(missing, &storage.FileInfo{Type: storage.TypeTable, Num: num})
				db.logf("db@janitor table missing @%d", num)
			}
		}
		return errors.NewErrCorrupted(nil, &errors.ErrMissingFiles{Files: missing})
	}

	db.logf("db@janitor F·%d G·%d", len(files), len(rem))
	for _, f := range rem {
		db.logf("db@janitor removing %s-%d", f.Type(), f.Num())
		if err := f.Remove(); err != nil {
			return err
		}
	}
	return nil
}

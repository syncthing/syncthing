// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import (
	"errors"

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

// Check and clean files.
func (d *DB) checkAndCleanFiles() error {
	s := d.s

	v := s.version_NB()
	tables := make(map[uint64]bool)
	for _, tt := range v.tables {
		for _, t := range tt {
			tables[t.file.Num()] = false
		}
	}

	ff, err := s.getFiles(storage.TypeAll)
	if err != nil {
		return err
	}

	var nTables int
	var rem []storage.File
	for _, f := range ff {
		keep := true
		switch f.Type() {
		case storage.TypeManifest:
			keep = f.Num() >= s.manifestFile.Num()
		case storage.TypeJournal:
			if d.frozenJournalFile != nil {
				keep = f.Num() >= d.frozenJournalFile.Num()
			} else {
				keep = f.Num() >= d.journalFile.Num()
			}
		case storage.TypeTable:
			_, keep = tables[f.Num()]
			if keep {
				tables[f.Num()] = true
				nTables++
			}
		}

		if !keep {
			rem = append(rem, f)
		}
	}

	if nTables != len(tables) {
		for num, present := range tables {
			if !present {
				s.logf("db@janitor table missing @%d", num)
			}
		}
		return ErrCorrupted{Type: MissingFiles, Err: errors.New("leveldb: table files missing")}
	}

	s.logf("db@janitor F·%d G·%d", len(ff), len(rem))
	for _, f := range rem {
		s.logf("db@janitor removing %s-%d", f.Type(), f.Num())
		if err := f.Remove(); err != nil {
			return err
		}
	}
	return nil
}

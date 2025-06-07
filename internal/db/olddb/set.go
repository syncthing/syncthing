// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package db provides a set type to track local/remote files with newness
// checks. We must do a certain amount of normalization in here. We will get
// fed paths with either native or wire-format separators and encodings
// depending on who calls us. We transform paths to wire-format (NFC and
// slashes) on the way to the database, and transform to native format
// (varying separator and encoding) on the way back out.
package olddb

import (
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/protocol"
)

type deprecatedFileSet struct {
	folder string
	db     *deprecatedLowlevel
}

// The Iterator is called with either a protocol.FileInfo or a
// FileInfoTruncated (depending on the method) and returns true to
// continue iteration, false to stop.
type Iterator func(f protocol.FileInfo) bool

func NewFileSet(folder string, db *deprecatedLowlevel) (*deprecatedFileSet, error) {
	s := &deprecatedFileSet{
		folder: folder,
		db:     db,
	}
	return s, nil
}

type Snapshot struct {
	folder string
	t      readOnlyTransaction
}

func (s *deprecatedFileSet) Snapshot() (*Snapshot, error) {
	t, err := s.db.newReadOnlyTransaction()
	if err != nil {
		return nil, err
	}
	return &Snapshot{
		folder: s.folder,
		t:      t,
	}, nil
}

func (s *Snapshot) Release() {
	s.t.close()
}

func (s *Snapshot) WithHaveSequence(startSeq int64, fn Iterator) error {
	return s.t.withHaveSequence([]byte(s.folder), startSeq, nativeFileIterator(fn))
}

func nativeFileIterator(fn Iterator) Iterator {
	return func(fi protocol.FileInfo) bool {
		fi.Name = osutil.NativeFilename(fi.Name)
		return fn(fi)
	}
}

// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"encoding/binary"
	"fmt"

	"github.com/syncthing/syncthing/lib/osutil"
)

type BlockFinder struct {
	db *Lowlevel
}

func NewBlockFinder(db *Lowlevel) *BlockFinder {
	return &BlockFinder{
		db: db,
	}
}

func (f *BlockFinder) String() string {
	return fmt.Sprintf("BlockFinder@%p", f)
}

// Iterate takes an iterator function which iterates over all matching blocks
// for the given hash. The iterator function has to return either true (if
// they are happy with the block) or false to continue iterating for whatever
// reason. The iterator finally returns the result, whether or not a
// satisfying block was eventually found.
func (f *BlockFinder) Iterate(folders []string, hash []byte, iterFn func(string, string, int32) bool) bool {
	t, err := f.db.newReadOnlyTransaction()
	if err != nil {
		return false
	}
	defer t.close()

	var key []byte
	for _, folder := range folders {
		key, err = f.db.keyer.GenerateBlockMapKey(key, []byte(folder), hash, nil)
		if err != nil {
			return false
		}
		iter, err := t.NewPrefixIterator(key)
		if err != nil {
			return false
		}

		for iter.Next() && iter.Error() == nil {
			file := string(f.db.keyer.NameFromBlockMapKey(iter.Key()))
			index := int32(binary.BigEndian.Uint32(iter.Value()))
			if iterFn(folder, osutil.NativeFilename(file), index) {
				iter.Release()
				return true
			}
		}
		iter.Release()
	}
	return false
}

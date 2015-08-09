// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package db

import (
	"fmt"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
)

// This type encapsulates a repository of mtimes for platforms where file mtimes
// can't be set to arbitrary values.  For this to work, we need to store both
// the mtime we tried to set (the "actual" mtime) as well as the mtime the file
// has when we're done touching it (the "disk" mtime) so that we can tell if it
// was changed.  So in GetMtime(), it's not sufficient that the record exists --
// the argument must also equal the "disk" mtime in the record, otherwise it's
// been touched locally and the "disk" mtime is actually correct.

type VirtualMtimeRepo struct {
	ns *NamespacedKV
}

func NewVirtualMtimeRepo(ldb *leveldb.DB, folder string) *VirtualMtimeRepo {
	prefix := string(KeyTypeVirtualMtime) + folder

	return &VirtualMtimeRepo{
		ns: NewNamespacedKV(ldb, prefix),
	}
}

func (r *VirtualMtimeRepo) UpdateMtime(path string, diskMtime, actualMtime time.Time) {
	if debug {
		l.Debugf("virtual mtime: storing values for path:%s disk:%v actual:%v", path, diskMtime, actualMtime)
	}

	diskBytes, _ := diskMtime.MarshalBinary()
	actualBytes, _ := actualMtime.MarshalBinary()

	data := append(diskBytes, actualBytes...)

	r.ns.PutBytes(path, data)
}

func (r *VirtualMtimeRepo) GetMtime(path string, diskMtime time.Time) time.Time {
	data, exists := r.ns.Bytes(path)
	if !exists {
		// Absense of debug print is significant enough in itself here
		return diskMtime
	}

	var mtime time.Time
	if err := mtime.UnmarshalBinary(data[:len(data)/2]); err != nil {
		panic(fmt.Sprintf("Can't unmarshal stored mtime at path %s: %v", path, err))
	}

	if mtime.Equal(diskMtime) {
		if err := mtime.UnmarshalBinary(data[len(data)/2:]); err != nil {
			panic(fmt.Sprintf("Can't unmarshal stored mtime at path %s: %v", path, err))
		}

		if debug {
			l.Debugf("virtual mtime: return %v instead of %v for path: %s", mtime, diskMtime, path)
		}
		return mtime
	}

	if debug {
		l.Debugf("virtual mtime: record exists, but mismatch inDisk: %v dbDisk: %v for path: %s", diskMtime, mtime, path)
	}
	return diskMtime
}

func (r *VirtualMtimeRepo) DeleteMtime(path string) {
	r.ns.Delete(path)
}

func (r *VirtualMtimeRepo) Drop() {
	r.ns.Reset()
}

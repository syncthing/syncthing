// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package fs

import (
	"fmt"
	"os"
	"time"
)

// An MtimeFS provides os.Chtimes- and os.Lstat-equivalents that may use an
// underlying store to stash modification times when these cannot be changed
// directly on disk.
type MtimeFS struct {
	store ByteStore
	Filesystem
}

// The ByteStore provides a persistent map[string][]byte, used to store
// virtual mtimes.
type ByteStore interface {
	PutBytes(path string, bytes []byte)
	Bytes(path string) ([]byte, bool)
}

func NewMtimeFS(store ByteStore) *MtimeFS {
	return &MtimeFS{
		store:      store,
		Filesystem: DefaultFilesystem,
	}
}

// Chtimes attemps to update the modification time on disk. Should this fail,
// it will record the current mtime and the desired mtime in the virtual
// mtime store instead.
func (r *MtimeFS) Chtimes(path string, atime, mtime time.Time) error {
	if err := r.Filesystem.Chtimes(path, atime, mtime); err == nil {
		// It worked, we're done!
		return nil
	}

	// Figure out the on disk mtime
	info, err := r.Filesystem.Lstat(path)
	if err != nil {
		// This will be the ENOTEXIST and similar that we should return as
		// usual.
		return err
	}

	// Store the mtime as it should have been
	diskBytes, _ := info.ModTime().MarshalBinary()
	actualBytes, _ := mtime.MarshalBinary()
	data := append(diskBytes, actualBytes...)
	r.store.PutBytes(path, data)
	return nil
}

// Lstat performs an os.Lstat() but the returned FileInfo may have an mtime
// overridden by the virtual mtime store.
func (r *MtimeFS) Lstat(path string) (os.FileInfo, error) {
	info, err := r.Filesystem.Lstat(path)
	if err != nil {
		return nil, err
	}

	data, ok := r.store.Bytes(path)
	if !ok {
		// We have nothing in the virtual store, so return the from-disk
		// result
		return info, nil
	}

	var mtime time.Time
	if err := mtime.UnmarshalBinary(data[:len(data)/2]); err != nil {
		panic(fmt.Sprintf("Can't unmarshal stored mtime at path %s: %v", path, err))
	}

	if mtime.Equal(info.ModTime()) {
		// The time on disk matches the on in our virtual store; we should
		// replace it.
		if err := mtime.UnmarshalBinary(data[len(data)/2:]); err != nil {
			panic(fmt.Sprintf("Can't unmarshal stored mtime at path %s: %v", path, err))
		}

		info = mtimeOverride{
			FileInfo: info,
			mtime:    mtime,
		}
	}

	return info, nil
}

// Stat performs an os.Stat() but the returned FileInfo may have an mtime
// overridden by the virtual mtime store.
func (r *MtimeFS) Stat(path string) (os.FileInfo, error) {
	info, err := r.Filesystem.Stat(path)
	if err != nil {
		return nil, err
	}

	data, ok := r.store.Bytes(path)
	if !ok {
		// We have nothing in the virtual store, so return the from-disk
		// result
		return info, nil
	}

	var mtime time.Time
	if err := mtime.UnmarshalBinary(data[:len(data)/2]); err != nil {
		panic(fmt.Sprintf("Can't unmarshal stored mtime at path %s: %v", path, err))
	}

	if mtime.Equal(info.ModTime()) {
		// The time on disk matches the on in our virtual store; we should
		// replace it.
		if err := mtime.UnmarshalBinary(data[len(data)/2:]); err != nil {
			panic(fmt.Sprintf("Can't unmarshal stored mtime at path %s: %v", path, err))
		}

		info = mtimeOverride{
			FileInfo: info,
			mtime:    mtime,
		}
	}

	return info, nil
}

// mtimeOverride wraps an os.FileInfo to provide an os.FileInfo that lies
// about the mtime.
type mtimeOverride struct {
	os.FileInfo
	mtime time.Time
}

func (m mtimeOverride) ModTime() time.Time {
	return m.mtime
}

// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"time"
)

// The database is where we store the virtual mtimes
type database interface {
	Bytes(key string) (data []byte, ok bool, err error)
	PutBytes(key string, data []byte) error
	Delete(key string) error
}

type mtimeFS struct {
	Filesystem
	chtimes         func(string, time.Time, time.Time) error
	db              database
	caseInsensitive bool
}

type MtimeFSOption func(*mtimeFS)

func WithCaseInsensitivity(v bool) MtimeFSOption {
	return func(f *mtimeFS) {
		f.caseInsensitive = v
	}
}

// NewMtimeFS returns a filesystem with nanosecond mtime precision, regardless
// of what shenanigans the underlying filesystem gets up to.
func NewMtimeFS(fs Filesystem, db database, options ...MtimeFSOption) Filesystem {
	return wrapFilesystem(fs, func(underlying Filesystem) Filesystem {
		f := &mtimeFS{
			Filesystem: underlying,
			chtimes:    underlying.Chtimes, // for mocking it out in the tests
			db:         db,
		}
		for _, opt := range options {
			opt(f)
		}
		return f
	})
}

func (f *mtimeFS) Chtimes(name string, atime, mtime time.Time) error {
	// Do a normal Chtimes call, don't care if it succeeds or not.
	f.chtimes(name, atime, mtime)

	// Stat the file to see what happened. Here we *do* return an error,
	// because it might be "does not exist" or similar.
	info, err := f.Filesystem.Lstat(name)
	if err != nil {
		return err
	}

	f.save(name, info.ModTime(), mtime)
	return nil
}

func (f *mtimeFS) Stat(name string) (FileInfo, error) {
	info, err := f.Filesystem.Stat(name)
	if err != nil {
		return nil, err
	}

	real, virtual, err := f.load(name)
	if err != nil {
		return nil, err
	}
	if real == info.ModTime() {
		info = mtimeFileInfo{
			FileInfo: info,
			mtime:    virtual,
		}
	}

	return info, nil
}

func (f *mtimeFS) Lstat(name string) (FileInfo, error) {
	info, err := f.Filesystem.Lstat(name)
	if err != nil {
		return nil, err
	}

	real, virtual, err := f.load(name)
	if err != nil {
		return nil, err
	}
	if real == info.ModTime() {
		info = mtimeFileInfo{
			FileInfo: info,
			mtime:    virtual,
		}
	}

	return info, nil
}

func (f *mtimeFS) Walk(root string, walkFn WalkFunc) error {
	return f.Filesystem.Walk(root, func(path string, info FileInfo, err error) error {
		if info != nil {
			real, virtual, loadErr := f.load(path)
			if loadErr != nil && err == nil {
				// The iterator gets to deal with the error
				err = loadErr
			}
			if real == info.ModTime() {
				info = mtimeFileInfo{
					FileInfo: info,
					mtime:    virtual,
				}
			}
		}
		return walkFn(path, info, err)
	})
}

func (f *mtimeFS) Create(name string) (File, error) {
	fd, err := f.Filesystem.Create(name)
	if err != nil {
		return nil, err
	}
	return mtimeFile{fd, f}, nil
}

func (f *mtimeFS) Open(name string) (File, error) {
	fd, err := f.Filesystem.Open(name)
	if err != nil {
		return nil, err
	}
	return mtimeFile{fd, f}, nil
}

func (f *mtimeFS) OpenFile(name string, flags int, mode FileMode) (File, error) {
	fd, err := f.Filesystem.OpenFile(name, flags, mode)
	if err != nil {
		return nil, err
	}
	return mtimeFile{fd, f}, nil
}

// "real" is the on disk timestamp
// "virtual" is what want the timestamp to be

func (f *mtimeFS) save(name string, real, virtual time.Time) {
	if f.caseInsensitive {
		name = UnicodeLowercase(name)
	}

	if real.Equal(virtual) {
		// If the virtual time and the real on disk time are equal we don't
		// need to store anything.
		f.db.Delete(name)
		return
	}

	mtime := dbMtime{
		real:    real,
		virtual: virtual,
	}
	bs, _ := mtime.Marshal() // Can't fail
	f.db.PutBytes(name, bs)
}

func (f *mtimeFS) load(name string) (real, virtual time.Time, err error) {
	if f.caseInsensitive {
		name = UnicodeLowercase(name)
	}

	data, exists, err := f.db.Bytes(name)
	if err != nil {
		return time.Time{}, time.Time{}, err
	} else if !exists {
		return time.Time{}, time.Time{}, nil
	}

	var mtime dbMtime
	if err := mtime.Unmarshal(data); err != nil {
		return time.Time{}, time.Time{}, err
	}

	return mtime.real, mtime.virtual, nil
}

// The mtimeFileInfo is an os.FileInfo that lies about the ModTime().

type mtimeFileInfo struct {
	FileInfo
	mtime time.Time
}

func (m mtimeFileInfo) ModTime() time.Time {
	return m.mtime
}

type mtimeFile struct {
	File
	fs *mtimeFS
}

func (f mtimeFile) Stat() (FileInfo, error) {
	info, err := f.File.Stat()
	if err != nil {
		return nil, err
	}

	real, virtual, err := f.fs.load(f.Name())
	if err != nil {
		return nil, err
	}
	if real == info.ModTime() {
		info = mtimeFileInfo{
			FileInfo: info,
			mtime:    virtual,
		}
	}

	return info, nil
}

func (f mtimeFile) unwrap() File {
	return f.File
}

// The dbMtime is our database representation

type dbMtime struct {
	real    time.Time
	virtual time.Time
}

func (t *dbMtime) Marshal() ([]byte, error) {
	bs0, _ := t.real.MarshalBinary()
	bs1, _ := t.virtual.MarshalBinary()
	return append(bs0, bs1...), nil
}

func (t *dbMtime) Unmarshal(bs []byte) error {
	if err := t.real.UnmarshalBinary(bs[:len(bs)/2]); err != nil {
		return err
	}
	if err := t.virtual.UnmarshalBinary(bs[len(bs)/2:]); err != nil {
		return err
	}
	return nil
}

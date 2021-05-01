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

type MtimeFS struct {
	Filesystem
	chtimes         func(string, time.Time, time.Time) error
	db              database
	caseInsensitive bool
}

type MtimeFSOption func(*MtimeFS)

func WithCaseInsensitivity(v bool) MtimeFSOption {
	return func(f *MtimeFS) {
		f.caseInsensitive = v
	}
}

// NewMtimeFS returns a filesystem with nanosecond mtime precision, regardless
// of what shenanigans the underlying filesystem gets up to.
func NewMtimeFS(fs Filesystem, db database, options ...MtimeFSOption) Filesystem {
	return wrapFilesystem(fs, func(underlying Filesystem) Filesystem {
		f := &MtimeFS{
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

func (f *MtimeFS) Chtimes(name string, atime, mtime time.Time) error {
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

func (f *MtimeFS) Stat(name string) (FileInfo, error) {
	info, err := f.Filesystem.Stat(name)
	if err != nil {
		return nil, err
	}

	mtimeMapping, err := f.Load(name)
	if err != nil {
		return nil, err
	}
	if mtimeMapping.Real == info.ModTime() {
		info = mtimeFileInfo{
			FileInfo: info,
			mtime:    mtimeMapping.Virtual,
		}
	}

	return info, nil
}

func (f *MtimeFS) Lstat(name string) (FileInfo, error) {
	info, err := f.Filesystem.Lstat(name)
	if err != nil {
		return nil, err
	}

	mtimeMapping, err := f.Load(name)
	if err != nil {
		return nil, err
	}
	if mtimeMapping.Real == info.ModTime() {
		info = mtimeFileInfo{
			FileInfo: info,
			mtime:    mtimeMapping.Virtual,
		}
	}

	return info, nil
}

func (f *MtimeFS) Walk(root string, walkFn WalkFunc) error {
	return f.Filesystem.Walk(root, func(path string, info FileInfo, err error) error {
		if info != nil {
			mtimeMapping, loadErr := f.Load(path)
			if loadErr != nil && err == nil {
				// The iterator gets to deal with the error
				err = loadErr
			}
			if mtimeMapping.Real == info.ModTime() {
				info = mtimeFileInfo{
					FileInfo: info,
					mtime:    mtimeMapping.Virtual,
				}
			}
		}
		return walkFn(path, info, err)
	})
}

func (f *MtimeFS) Create(name string) (File, error) {
	fd, err := f.Filesystem.Create(name)
	if err != nil {
		return nil, err
	}
	return mtimeFile{fd, f}, nil
}

func (f *MtimeFS) Open(name string) (File, error) {
	fd, err := f.Filesystem.Open(name)
	if err != nil {
		return nil, err
	}
	return mtimeFile{fd, f}, nil
}

func (f *MtimeFS) OpenFile(name string, flags int, mode FileMode) (File, error) {
	fd, err := f.Filesystem.OpenFile(name, flags, mode)
	if err != nil {
		return nil, err
	}
	return mtimeFile{fd, f}, nil
}

func (f *MtimeFS) underlying() (Filesystem, bool) {
	return f.Filesystem, true
}

func (f *MtimeFS) wrapperType() FilesystemWrapperType {
	return FilesystemWrapperTypeMtime
}

func (f *MtimeFS) save(name string, real, virtual time.Time) {
	if f.caseInsensitive {
		name = UnicodeLowercase(name)
	}

	if real.Equal(virtual) {
		// If the virtual time and the real on disk time are equal we don't
		// need to store anything.
		f.db.Delete(name)
		return
	}

	mtime := MtimeMapping{
		Real:    real,
		Virtual: virtual,
	}
	bs, _ := mtime.Marshal() // Can't fail
	f.db.PutBytes(name, bs)
}

func (f *MtimeFS) Load(name string) (MtimeMapping, error) {
	if f.caseInsensitive {
		name = UnicodeLowercase(name)
	}

	data, exists, err := f.db.Bytes(name)
	if err != nil {
		return MtimeMapping{}, err
	} else if !exists {
		return MtimeMapping{}, nil
	}

	var mtime MtimeMapping
	if err := mtime.Unmarshal(data); err != nil {
		return MtimeMapping{}, err
	}
	return mtime, nil
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
	fs *MtimeFS
}

func (f mtimeFile) Stat() (FileInfo, error) {
	info, err := f.File.Stat()
	if err != nil {
		return nil, err
	}

	mtimeMapping, err := f.fs.Load(f.Name())
	if err != nil {
		return nil, err
	}
	if mtimeMapping.Real == info.ModTime() {
		info = mtimeFileInfo{
			FileInfo: info,
			mtime:    mtimeMapping.Virtual,
		}
	}

	return info, nil
}

// Used by copyRange to unwrap the real file to access the
func (f mtimeFile) unwrap() File {
	return f.File
}

// The MtimeMapping is our database representation
type MtimeMapping struct {
	// "Real" is the on disk timestamp
	Real time.Time `json:"real"`
	// "Virtual" is what want the timestamp to be
	Virtual time.Time `json:"virtual"`
}

func (t *MtimeMapping) Marshal() ([]byte, error) {
	bs0, _ := t.Real.MarshalBinary()
	bs1, _ := t.Virtual.MarshalBinary()
	return append(bs0, bs1...), nil
}

func (t *MtimeMapping) Unmarshal(bs []byte) error {
	if err := t.Real.UnmarshalBinary(bs[:len(bs)/2]); err != nil {
		return err
	}
	if err := t.Virtual.UnmarshalBinary(bs[len(bs)/2:]); err != nil {
		return err
	}
	return nil
}

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
	GetMtime(folder, name string) (ondisk, virtual time.Time)
	PutMtime(folder, name string, ondisk, virtual time.Time) error
	DeleteMtime(folder, name string) error
}

type mtimeFS struct {
	Filesystem

	chtimes         func(string, time.Time, time.Time) error
	db              database
	folderID        string
	caseInsensitive bool
}

type MtimeFSOption func(*mtimeFS)

func WithCaseInsensitivity(v bool) MtimeFSOption {
	return func(f *mtimeFS) {
		f.caseInsensitive = v
	}
}

type optionMtime struct {
	db       database
	folderID string
	options  []MtimeFSOption
}

// NewMtimeOption makes any filesystem provide nanosecond mtime precision,
// regardless of what shenanigans the underlying filesystem gets up to.
func NewMtimeOption(db database, folderID string, options ...MtimeFSOption) Option {
	return &optionMtime{
		db:       db,
		folderID: folderID,
		options:  options,
	}
}

func (o *optionMtime) apply(fs Filesystem) Filesystem {
	f := &mtimeFS{
		Filesystem: fs,
		chtimes:    fs.Chtimes, // for mocking it out in the tests
		db:         o.db,
		folderID:   o.folderID,
	}
	for _, opt := range o.options {
		opt(f)
	}
	return f
}

func (*optionMtime) String() string {
	return "mtime"
}

func (f *mtimeFS) Chtimes(name string, atime, mtime time.Time) error {
	// Do a normal Chtimes call, don't care if it succeeds or not.
	_ = f.chtimes(name, atime, mtime)

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

	ondisk, virtual := f.load(name)
	if ondisk.Equal(info.ModTime()) {
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

	ondisk, virtual := f.load(name)
	if ondisk.Equal(info.ModTime()) {
		info = mtimeFileInfo{
			FileInfo: info,
			mtime:    virtual,
		}
	}

	return info, nil
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

func (f *mtimeFS) underlying() (Filesystem, bool) {
	return f.Filesystem, true
}

func (f *mtimeFS) save(name string, ondisk, virtual time.Time) {
	if f.caseInsensitive {
		name = UnicodeLowercaseNormalized(name)
	}

	if ondisk.Equal(virtual) {
		// If the virtual time and the real on disk time are equal we don't
		// need to store anything.
		_ = f.db.DeleteMtime(f.folderID, name)
		return
	}

	_ = f.db.PutMtime(f.folderID, name, ondisk, virtual)
}

func (f *mtimeFS) load(name string) (ondisk, virtual time.Time) {
	if f.caseInsensitive {
		name = UnicodeLowercaseNormalized(name)
	}

	return f.db.GetMtime(f.folderID, name)
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

	ondisk, virtual := f.fs.load(f.Name())
	if ondisk.Equal(info.ModTime()) {
		info = mtimeFileInfo{
			FileInfo: info,
			mtime:    virtual,
		}
	}

	return info, nil
}

// Used by copyRange to unwrap to the real file and access SyscallConn
func (f mtimeFile) unwrap() File {
	return f.File
}

func GetMtimeMapping(fs Filesystem, file string) (ondisk, virtual time.Time) {
	fs, ok := unwrapFilesystem[*mtimeFS](fs)
	if !ok {
		return time.Time{}, time.Time{}
	}
	mtimeFs, ok := fs.(*mtimeFS)
	if !ok {
		return time.Time{}, time.Time{}
	}
	return mtimeFs.load(file)
}

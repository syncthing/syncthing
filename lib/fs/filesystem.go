// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"io"
	"os"
	"path/filepath"
	"time"
)

// The Filesystem interface abstracts access to the file system.
type Filesystem interface {
	Chmod(name string, mode FileMode) error
	Chtimes(name string, atime time.Time, mtime time.Time) error
	Create(name string) (File, error)
	CreateSymlink(name, target string) error
	DirNames(name string) ([]string, error)
	Lstat(name string) (FileInfo, error)
	Mkdir(name string, perm FileMode) error
	Open(name string) (File, error)
	ReadSymlink(name string) (string, error)
	Remove(name string) error
	Rename(oldname, newname string) error
	Stat(name string) (FileInfo, error)
	SymlinksSupported() bool
	Walk(root string, walkFn WalkFunc) error
}

// The File interface abstracts access to a regular file, being a somewhat
// smaller interface than os.File
type File interface {
	io.Reader
	io.WriterAt
	io.Closer
	Truncate(size int64) error
	Stat() (FileInfo, error)
}

// The FileInfo interface is almost the same as os.FileInfo, but with the
// Sys method removed (as we don't want to expose whatever is underlying)
// and with a couple of convenience methods added.
type FileInfo interface {
	// Standard things present in os.FileInfo
	Name() string
	Mode() FileMode
	Size() int64
	ModTime() time.Time
	IsDir() bool
	// Extensions
	IsRegular() bool
	IsSymlink() bool
}

// FileMode is similar to os.FileMode
type FileMode uint32

// ModePerm is the equivalent of os.ModePerm
const ModePerm = FileMode(os.ModePerm)

// DefaultFilesystem is the fallback to use when nothing explicitly has
// been passed.
var DefaultFilesystem Filesystem = NewWalkFilesystem(NewBasicFilesystem())

// SkipDir is used as a return value from WalkFuncs to indicate that
// the directory named in the call is to be skipped. It is not returned
// as an error by any function.
var SkipDir = filepath.SkipDir

// IsExist is the equivalent of os.IsExist
var IsExist = os.IsExist

// IsNotExist is the equivalent of os.IsNotExist
var IsNotExist = os.IsNotExist

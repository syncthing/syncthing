// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package fs

import (
	"io"
	"os"
	"time"
)

type LinkTargetType int

const (
	LinkTargetFile LinkTargetType = iota
	LinkTargetDirectory
	LinkTargetUnknown
)

// The Filesystem interface abstracts access to the file system.
type Filesystem interface {
	Chmod(name string, mode os.FileMode) error
	Chtimes(name string, atime time.Time, mtime time.Time) error
	Lstat(name string) (os.FileInfo, error)
	Mkdir(path string, perm os.FileMode) error
	Remove(name string) error
	Rename(oldpath, newpath string) error
	Stat(name string) (os.FileInfo, error)
	DirNames(path string) ([]string, error)
	OpenFile(path string, flag int) (File, error)
	SymlinksSupported() bool
	CreateSymlink(path, target string, tt LinkTargetType) error
	ChangeSymlinkType(path string, tt LinkTargetType) error
	ReadSymlink(path string) (string, LinkTargetType, error)
}

type File interface {
	io.WriterAt
	io.Closer
	Truncate(size int64) error
}

var DefaultFilesystem = ExtendedFilesystem{BasicFilesystem{}}

// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package fs

import (
	"os"
	"time"
)

// The BasicFilesystem implements all aspects by delegating to package os.
type BasicFilesystem struct {
}

func NewBasicFilesystem() *BasicFilesystem {
	return new(BasicFilesystem)
}

func (f *BasicFilesystem) Chmod(name string, mode FileMode) error {
	return os.Chmod(name, os.FileMode(mode))
}

func (f *BasicFilesystem) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return os.Chtimes(name, atime, mtime)
}

func (f *BasicFilesystem) Mkdir(name string, perm FileMode) error {
	return os.Mkdir(name, os.FileMode(perm))
}

func (f *BasicFilesystem) Lstat(name string) (FileInfo, error) {
	fi, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	return fsFileInfo{fi}, err
}

func (f *BasicFilesystem) Remove(name string) error {
	return os.Remove(name)
}

func (f *BasicFilesystem) Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

func (f *BasicFilesystem) Stat(name string) (FileInfo, error) {
	fi, err := os.Stat(name)
	if err != nil {
		return nil, err
	}
	return fsFileInfo{fi}, err
}

func (f *BasicFilesystem) DirNames(name string) ([]string, error) {
	fd, err := os.OpenFile(name, os.O_RDONLY, 0777)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	names, err := fd.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	return names, nil
}

func (f *BasicFilesystem) Open(name string) (File, error) {
	return os.Open(name)
}

func (f *BasicFilesystem) Create(name string) (File, error) {
	return os.Create(name)
}

// fsFileInfo implements the fs.FileInfo interface on top of an os.FileInfo.
type fsFileInfo struct {
	os.FileInfo
}

func (e fsFileInfo) Mode() FileMode {
	return FileMode(e.FileInfo.Mode())
}

func (e fsFileInfo) IsRegular() bool {
	return e.FileInfo.Mode().IsRegular()
}

func (e fsFileInfo) IsSymlink() bool {
	return e.FileInfo.Mode()&os.ModeSymlink == os.ModeSymlink
}

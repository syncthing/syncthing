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
type BasicFilesystem struct{}

func (BasicFilesystem) Chmod(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}

func (BasicFilesystem) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return os.Chtimes(name, atime, mtime)
}

func (BasicFilesystem) Mkdir(path string, perm os.FileMode) error {
	return os.Mkdir(path, perm)
}

func (BasicFilesystem) Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(name)
}

func (BasicFilesystem) Remove(name string) error {
	return os.Remove(name)
}

func (BasicFilesystem) Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

func (BasicFilesystem) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (BasicFilesystem) DirNames(path string) ([]string, error) {
	fd, err := os.OpenFile(path, os.O_RDONLY, 0777)
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

func (BasicFilesystem) OpenFile(path string, flag int) (File, error) {
	return os.OpenFile(path, flag, 0666)
}

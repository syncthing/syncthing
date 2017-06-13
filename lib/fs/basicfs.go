// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

// The BasicFilesystem implements all aspects by delegating to package os.
type BasicFilesystem struct {
	root string
}

func NewBasicFilesystem(root string) *BasicFilesystem {
	return &BasicFilesystem{
		root: root,
	}
}

// rooted roots the path at the root of the filesystem
func (f *BasicFilesystem) rooted(name string) string {
	return filepath.Join(f.root, name)
}

func (f *BasicFilesystem) Chmod(name string, mode FileMode) error {
	return os.Chmod(f.rooted(name), os.FileMode(mode))
}

func (f *BasicFilesystem) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return os.Chtimes(f.rooted(name), atime, mtime)
}

func (f *BasicFilesystem) Mkdir(name string, perm FileMode) error {
	return os.Mkdir(f.rooted(name), os.FileMode(perm))
}

func (f *BasicFilesystem) Lstat(name string) (FileInfo, error) {
	fi, err := underlyingLstat(f.rooted(name))
	if err != nil {
		return nil, err
	}
	return fsFileInfo{fi}, err
}

func (f *BasicFilesystem) Remove(name string) error {
	return os.Remove(f.rooted(name))
}

func (f *BasicFilesystem) Rename(oldpath, newpath string) error {
	return os.Rename(f.rooted(oldpath), f.rooted(newpath))
}

func (f *BasicFilesystem) Stat(name string) (FileInfo, error) {
	fi, err := os.Stat(f.rooted(name))
	if err != nil {
		return nil, err
	}
	return fsFileInfo{fi}, err
}

func (f *BasicFilesystem) DirNames(name string) ([]string, error) {
	fd, err := os.OpenFile(f.rooted(name), os.O_RDONLY, 0777)
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
	fd, err := os.Open(f.rooted(name))
	if err != nil {
		return nil, err
	}
	return fsFile{fd}, err
}

func (f *BasicFilesystem) Create(name string) (File, error) {
	fd, err := os.Create(f.rooted(name))
	if err != nil {
		return nil, err
	}
	return fsFile{fd}, err
}

func (f *BasicFilesystem) Walk(root string, walkFn WalkFunc) error {
	// implemented in WalkFilesystem
	return errors.New("not implemented")
}

// fsFile implements the fs.File interface on top of an os.File
type fsFile struct {
	*os.File
}

func (f fsFile) Stat() (FileInfo, error) {
	info, err := f.File.Stat()
	if err != nil {
		return nil, err
	}
	return fsFileInfo{info}, nil
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

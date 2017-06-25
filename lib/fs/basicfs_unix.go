// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build !windows

package fs

import (
	"os"
	"runtime"
)

var symlinksSupported = true

func DisableSymlinks() {
	symlinksSupported = false
}

func (BasicFilesystem) SymlinksSupported() bool {
	return symlinksSupported
}

func (f *BasicFilesystem) CreateSymlink(name, target string) error {
	name, err := f.rooted(name)
	if err != nil {
		return err
	}
	target, err := f.rooted(target)
	if err != nil {
		return err
	}
	return os.Symlink(target, name)
}

func (f *BasicFilesystem) ReadSymlink(name string) (string, error) {
	name, err := f.rooted(name)
	if err != nil {
		return err
	}
	return os.Readlink(name)
}

func (f *BasicFilesystem) MkdirAll(name string, perm FileMode) error {
	name, err := f.rooted(name)
	if err != nil {
		return err
	}
	return os.MkdirAll(name, os.FileMode(perm))
}

func (f *BasicFilesystem) Show(name string) error {
	_, err := f.rooted(name)
	return err
}

func (f *BasicFilesystem) Hide(name string) error {
	_, err := f.rooted(name)
	return err
}

func (f *BasicFilesystem) SyncDir(name string) error {
	name, err := f.rooted(name)
	if err != nil {
		return nil, err
	}

	info, err := os.Lstat(name)
	if err != nil {
		return err
	}

	if info.IsDir() && runtime.GOOS == "windows" {
		// not supported by Windows
		return nil
	}

	flag := 0
	if runtime.GOOS == "windows" {
		flag = os.O_WRONLY
	}
	fd, err := os.OpenFile(name, flag, 0)
	if err != nil {
		return err
	}
	defer fd.Close()
	// MacOS and Windows do not flush the disk cache
	return fd.Sync()
}

func (f *BasicFilesystem) Roots() ([]string, error) {
	return []string{"/"}, nil
}

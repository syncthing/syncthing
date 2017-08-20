// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build !windows

package fs

import "os"

func (BasicFilesystem) SymlinksSupported() bool {
	return true
}

func (f *BasicFilesystem) CreateSymlink(target, name string) error {
	name, err := f.rooted(name)
	if err != nil {
		return err
	}
	return os.Symlink(target, name)
}

func (f *BasicFilesystem) ReadSymlink(name string) (string, error) {
	name, err := f.rooted(name)
	if err != nil {
		return "", err
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

// Unhide is a noop on unix, as unhiding files requires renaming them.
// We still check that the relative path does not try to escape the root
func (f *BasicFilesystem) Unhide(name string) error {
	_, err := f.rooted(name)
	return err
}

// Hide is a noop on unix, as hiding files requires renaming them.
// We still check that the relative path does not try to escape the root
func (f *BasicFilesystem) Hide(name string) error {
	_, err := f.rooted(name)
	return err
}

func (f *BasicFilesystem) Roots() ([]string, error) {
	return []string{"/"}, nil
}

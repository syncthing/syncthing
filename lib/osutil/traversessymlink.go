// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil

import (
	"path/filepath"

	"github.com/syncthing/syncthing/lib/fs"
)

// TraversesSymlinkError is an error indicating symlink traversal
type TraversesSymlinkError struct {
	path string
}

func (e *TraversesSymlinkError) Error() string {
	return "traverses symlink: " + e.path
}

// NotADirectoryError is an error indicating an expected path is not a directory
type NotADirectoryError struct {
	path string
}

func (e *NotADirectoryError) Error() string {
	return "not a directory: " + e.path
}

// TraversesSymlink returns an error if any path component of name (including name
// itself) traverses a symlink.
func TraversesSymlink(filesystem fs.Filesystem, name string) error {
	var err error
	name, err = fs.Canonicalize(name)
	if err != nil {
		return err
	}

	if name == "." {
		// The result of calling TraversesSymlink(filesystem, filepath.Dir("foo"))
		return nil
	}

	var path string
	for _, part := range fs.PathComponents(name) {
		path = filepath.Join(path, part)
		info, err := filesystem.Lstat(path)
		if err != nil {
			if fs.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsSymlink() {
			return &TraversesSymlinkError{
				path: path,
			}
		}
		if !info.IsDir() {
			return &NotADirectoryError{
				path: path,
			}
		}
	}
	return nil
}

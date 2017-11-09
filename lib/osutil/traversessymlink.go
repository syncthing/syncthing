// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/syncthing/syncthing/lib/fs"
)

// TraversesSymlinkError is an error indicating symlink traversal
type TraversesSymlinkError struct {
	path string
}

func (e TraversesSymlinkError) Error() string {
	return fmt.Sprintf("traverses symlink: %s", e.path)
}

// NotADirectoryError is an error indicating an expected path is not a directory
type NotADirectoryError struct {
	path string
}

func (e NotADirectoryError) Error() string {
	return fmt.Sprintf("not a directory: %s", e.path)
}

// TraversesSymlink returns an error and the part of path causing it, if base
// and any path component of name up to and including name traverses a symlink,
// is not a directory or is missing.
// Base and name must both be clean and name must be relative to base.
func TraversesSymlink(filesystem fs.Filesystem, name string) (error, string) {
	base := "."
	path := base
	info, err := filesystem.Lstat(path)
	if err != nil {
		return err, base
	}
	if !info.IsDir() {
		return &NotADirectoryError{
			path: base,
		}, base
	}

	if name == "." {
		// The result of calling TraversesSymlink("some/where", filepath.Dir("foo"))
		return nil, ""
	}

	parts := strings.Split(name, string(fs.PathSeparator))
	for _, part := range parts {
		path = filepath.Join(path, part)
		info, err := filesystem.Lstat(path)
		if err != nil {
			return err, path
		}
		if info.IsSymlink() {
			return &TraversesSymlinkError{
				path: strings.TrimPrefix(path, base),
			}, path
		}
		if !info.IsDir() {
			return &NotADirectoryError{
				path: strings.TrimPrefix(path, base),
			}, path
		}
	}

	return nil, ""
}

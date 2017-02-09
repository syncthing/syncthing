// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// TraversesSymlink returns an error if base and any path component of name up to and
// including filepath.Join(base, name) traverses a symlink.
// Base and name must both be clean and name must be relative to base.
func TraversesSymlink(base, name string) error {
	path := base
	info, err := Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return &NotADirectoryError{
			path: base,
		}
	}

	if name == "." {
		// The result of calling TraversesSymlink("some/where", filepath.Dir("foo"))
		return nil
	}

	parts := strings.Split(name, string(os.PathSeparator))
	for _, part := range parts {
		path = filepath.Join(path, part)
		info, err := Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return &TraversesSymlinkError{
				path: strings.TrimPrefix(path, base),
			}
		}
		if !info.IsDir() {
			return &NotADirectoryError{
				path: strings.TrimPrefix(path, base),
			}
		}
	}
	return nil
}

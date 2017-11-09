// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/syncthing/syncthing/lib/fs"
)

var ErrTraversesSymlink error = errors.New("traverses symlink")
var ErrTraversesNotADirectory error = errors.New("traverses something that is not a direcotry")

type TraverseError struct {
	Err  error
	Path string
}

func (e TraverseError) Error() string { return e.Err.Error() + ": " + e.Path }

// TraversesSymlink returns a TraverseError, if any path component of name up to
// and including name traverses a symlink, is not a directory or is missing.
// Name must be clean.
func TraversesSymlink(filesystem fs.Filesystem, name string) *TraverseError {
	base := "."
	path := base
	info, err := filesystem.Lstat(path)
	if err != nil {
		return &TraverseError{
			Err:  err,
			Path: path,
		}
	}
	if !info.IsDir() {
		return &TraverseError{
			Err:  ErrTraversesNotADirectory,
			Path: path,
		}
	}

	if name == "." {
		// The result of calling TraversesSymlink("some/where", filepath.Dir("foo"))
		return nil
	}

	parts := strings.Split(name, string(fs.PathSeparator))
	for _, part := range parts {
		path = filepath.Join(path, part)
		info, err := filesystem.Lstat(path)
		if err != nil {
			return &TraverseError{
				Err:  err,
				Path: path,
			}
		}
		if info.IsSymlink() {
			return &TraverseError{
				Err:  ErrTraversesSymlink,
				Path: path,
			}
		}
		if !info.IsDir() {
			return &TraverseError{
				Err:  ErrTraversesNotADirectory,
				Path: path,
			}
		}
	}

	return nil
}

// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

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

// TraversesSymlink returns an error if base and any path component of name up to and
// including filepath.Join(base, name) traverses a symlink.
// Base and name must both be clean and name must be relative to base.
func TraversesSymlink(base, name string) error {
	baseResolved, err := filepath.EvalSymlinks(base)
	if err != nil {
		return err
	}

	fullName := filepath.Join(baseResolved, name)
	fullNameResolved, err := filepath.EvalSymlinks(fullName)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if fullNameResolved != fullName {
		return &TraversesSymlinkError{
			path: strings.TrimPrefix(fullNameResolved, baseResolved),
		}
	}

	return nil
}

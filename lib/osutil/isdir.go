// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package osutil

import (
	"os"
	"path/filepath"
	"strings"
)

// IsDir returns true if base and every path component of name up to and
// including filepath.Join(base, name) is a directory (and not a symlink or
// similar). Base and name must both be clean and name must be relative to
// base.
func IsDir(base, name string) bool {
	path := base
	info, err := Lstat(path)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return false
	}

	if name == "." {
		// The result of calling IsDir("some/where", filepath.Dir("foo"))
		return true
	}

	parts := strings.Split(name, string(os.PathSeparator))
	for _, part := range parts {
		path = filepath.Join(path, part)
		info, err := Lstat(path)
		if err != nil {
			return false
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		if !info.IsDir() {
			return false
		}
	}
	return true
}

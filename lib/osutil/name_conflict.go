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

// CheckNameConflict returns true if every path component of name up to and
// including filepath.Join(base, name) doesn't conflict with any existing
// files or folders with different names. Base and name must both be clean and
// name must be relative to base.
func CheckNameConflict(base, name string) bool {
	// Conflicts depend on the OS and file system.
	subname := "."
	parts := strings.Split(name, string(os.PathSeparator))
	for _, part := range parts {
		subname = filepath.Join(subname, part)
		fileName, err := FindRealFileName(base, subname)
		if err != nil {
			return false
		}
		if fileName == "" {
			// doesn't exist
			return true
		}
		if fileName != part {
			// conflict
			return false
		}
	}
	return true
}

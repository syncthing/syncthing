// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build windows

package osutil

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// CheckNameConflict returns true if every path component of name up to and
// including filepath.Join(base, name) doesn't conflict with any existing
// files or folders with different names. Base and name must both be clean and
// name must be relative to base.
func CheckNameConflict(base, name string) bool {
	// Conflicts can be caused by different casing (e.g. foo and FOO) or
	// by the use of short names (e.g. foo.barbaz and FOO~1.BAR).
	path := base
	parts := strings.Split(name, string(os.PathSeparator))
	for _, part := range parts {
		path = filepath.Join(path, part)
		pathp, err := syscall.UTF16PtrFromString(path)
		if err != nil {
			return false
		}
		var data syscall.Win32finddata
		handle, err := syscall.FindFirstFile(pathp, &data)
		if err == syscall.ERROR_FILE_NOT_FOUND {
			return true
		}
		if err != nil {
			return false
		}
		syscall.Close(handle)
		fileName := syscall.UTF16ToString(data.FileName[:])
		if part != fileName {
			return false
		}
	}
	return true
}

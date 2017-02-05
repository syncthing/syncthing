// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build windows

package osutil

import (
	"path/filepath"
	"syscall"
)

// FindRealFileName returns the real name of the last path component of name.
// Base and name must both be clean and name must be relative to base.
// If the last path component of name doesn't exist "" is returned.
func FindRealFileName(base, name string) (string, error) {
	// Conflicts can be caused by different casing (e.g. foo and FOO) or
	// by the use of short names (e.g. foo.barbaz and FOO~1.BAR).
	path := filepath.Join(base, name)
	pathp, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	var data syscall.Win32finddata
	handle, err := syscall.FindFirstFile(pathp, &data)
	if err == syscall.ERROR_FILE_NOT_FOUND {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	syscall.FindClose(handle)
	return syscall.UTF16ToString(data.FileName[:]), nil
}

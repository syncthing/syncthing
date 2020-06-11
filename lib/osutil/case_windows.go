// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil

import (
	"path/filepath"
	"strings"
	"syscall"

	"github.com/syncthing/syncthing/lib/fs"
)

// RealCase returns the correct case for the given name, which is a relative
// path below root, as it exists on disk.
func RealCase(ffs fs.Filesystem, name string) (string, error) {
	path := ffs.URI()
	comps := strings.Split(name, string(fs.PathSeparator))
	var err error
	for i, comp := range comps {
		path = filepath.Join(path, comp)
		comps[i], err = realCaseBase(path)
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(comps...), nil
}

func realCaseBase(path string) (string, error) {
	p, err := syscall.UTF16PtrFromString(fixLongPath(path))
	if err != nil {
		return "", err
	}
	var fd syscall.Win32finddata
	h, err := syscall.FindFirstFile(p, &fd)
	if err != nil {
		return "", err
	}
	syscall.FindClose(h)
	return syscall.UTF16ToString(fd.FileName[:]), nil
}

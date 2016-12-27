// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build !windows

package osutil

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// FindRealFileName returns the real name of the last path component of name.
// Base and name must both be clean and name must be relative to base.
// If the last path component of name doesn't exist "" is returned.
func FindRealFileName(base, name string) (string, error) {
	// Conflicts can be caused by different casing (e.g. foo and FOO).
	info, err := os.Lstat(filepath.Join(base, name))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", errors.New("Not a syscall.Stat_t")
	}
	targetIno := stat.Ino
	fd, err := os.Open(filepath.Join(base, filepath.Dir(name)))
	if err != nil {
		// possible race condition
		return "", err
	}
	defer fd.Close()
	for {
		infos, err := fd.Readdir(256)
		if err != nil {
			// possible race condition
			return "", err
		}
		for _, info := range infos {
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok {
				return "", errors.New("Not a syscall.Stat_t")
			}
			if stat.Ino == targetIno {
				return info.Name(), nil
			}
		}
	}
}

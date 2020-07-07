// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build windows

package fs

import (
	"path/filepath"
	"strings"
	"syscall"
)

type basicRealCaserWindows struct {
	uri string
}

func newBasicRealCaser(fs Filesystem) realCaser {
	return &basicRealCaserWindows{fs.URI()}
}

// RealCase returns the correct case for the given name, which is a relative
// path below root, as it exists on disk.
func (r *basicRealCaserWindows) realCase(name string) (string, error) {
	if name == "." {
		return ".", nil
	}
	path := r.uri
	comps := strings.Split(name, string(PathSeparator))
	var err error
	for i, comp := range comps {
		path = filepath.Join(path, comp)
		comps[i], err = r.realCaseBase(path)
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(comps...), nil
}

func (*basicRealCaserWindows) realCaseBase(path string) (string, error) {
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

func (r *basicRealCaserWindows) dropCache() {}

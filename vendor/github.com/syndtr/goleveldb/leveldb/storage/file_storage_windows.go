// Copyright (c) 2013, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"syscall"
	"unsafe"
)

var (
	modkernel32 = syscall.NewLazyDLL("kernel32.dll")

	procMoveFileExW = modkernel32.NewProc("MoveFileExW")
)

const (
	_MOVEFILE_REPLACE_EXISTING = 1
)

type windowsFileLock struct {
	fd syscall.Handle
}

func (fl *windowsFileLock) release() error {
	return syscall.Close(fl.fd)
}

func newFileLock(path string, readOnly bool) (fl fileLock, err error) {
	pathp, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	var access, shareMode uint32
	if readOnly {
		access = syscall.GENERIC_READ
		shareMode = syscall.FILE_SHARE_READ
	} else {
		access = syscall.GENERIC_READ | syscall.GENERIC_WRITE
	}
	fd, err := syscall.CreateFile(pathp, access, shareMode, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err == syscall.ERROR_FILE_NOT_FOUND {
		fd, err = syscall.CreateFile(pathp, access, shareMode, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	}
	if err != nil {
		return
	}
	fl = &windowsFileLock{fd: fd}
	return
}

func moveFileEx(from *uint16, to *uint16, flags uint32) error {
	r1, _, e1 := syscall.Syscall(procMoveFileExW.Addr(), 3, uintptr(unsafe.Pointer(from)), uintptr(unsafe.Pointer(to)), uintptr(flags))
	if r1 == 0 {
		if e1 != 0 {
			return error(e1)
		}
		return syscall.EINVAL
	}
	return nil
}

func rename(oldpath, newpath string) error {
	from, err := syscall.UTF16PtrFromString(oldpath)
	if err != nil {
		return err
	}
	to, err := syscall.UTF16PtrFromString(newpath)
	if err != nil {
		return err
	}
	return moveFileEx(from, to, _MOVEFILE_REPLACE_EXISTING)
}

func syncDir(name string) error { return nil }

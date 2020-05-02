// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build windows

package fs

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

//
// Most of the code below has been extracted from internal Golang packages.
// As the code in internal packages is uncallable, we had to copy or adapt the code here.
//

var (
	_                                unsafe.Pointer
	modkernel32                      = syscall.NewLazyDLL("kernel32.dll")
	procGetFileInformationByHandleEx = modkernel32.NewProc("GetFileInformationByHandleEx")
)

func GetFileInformationByHandleEx(handle syscall.Handle, class uint32, info *byte, bufsize uint32) (err error) {
	r1, _, e1 := syscall.Syscall6(procGetFileInformationByHandleEx.Addr(), 4, uintptr(handle), uintptr(class), uintptr(unsafe.Pointer(info)), uintptr(bufsize), 0, 0)
	if r1 == 0 {
		if e1 != 0 {
			err = e1
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func fixLongPath(path string) string {
	//TODO: extract the implementation from https://golang.org/src/os/path_windows.go; probably not necessary
	return path
}

const (
	ERROR_INVALID_PARAMETER    syscall.Errno = 87
	FileAttributeTagInfo                     = 9 // FILE_ATTRIBUTE_TAG_INFO
	IO_REPARSE_TAG_MOUNT_POINT               = 0xA0000003
	IO_REPARSE_TAG_SYMLINK                   = 0xA000000C
)

type FILE_ATTRIBUTE_TAG_INFO struct {
	FileAttributes uint32
	ReparseTag     uint32
}

func isDirectoryJunction(path string) (bool, error) {
	namep, err := syscall.UTF16PtrFromString(fixLongPath(path))
	if err != nil {
		return false, errors.New(fmt.Sprintf("UTF16PtrFromString ERROR: %s\n", err))
	}
	attrs := uint32(syscall.FILE_FLAG_BACKUP_SEMANTICS | syscall.FILE_FLAG_OPEN_REPARSE_POINT)
	h, err := syscall.CreateFile(namep, 0, 0, nil, syscall.OPEN_EXISTING, attrs, 0)
	if err != nil {
		return false, errors.New(fmt.Sprintf("syscall.CreateFile ERROR: %s\n", err))
	}
	defer syscall.CloseHandle(h)

	var ti FILE_ATTRIBUTE_TAG_INFO
	err = GetFileInformationByHandleEx(h, FileAttributeTagInfo, (*byte)(unsafe.Pointer(&ti)), uint32(unsafe.Sizeof(ti)))
	if err != nil {
		if errno, ok := err.(syscall.Errno); ok && errno == ERROR_INVALID_PARAMETER {
			// It appears calling GetFileInformationByHandleEx with
			// FILE_ATTRIBUTE_TAG_INFO fails on FAT file system with
			// ERROR_INVALID_PARAMETER. Clear ti.ReparseTag in that
			// instance to indicate no symlinks are possible.
			ti.ReparseTag = 0
		} else {
			return false, errors.New(fmt.Sprintf("GetFileInformationByHandleEx ERROR: %s\n", err))
		}
	}
	return ti.ReparseTag == IO_REPARSE_TAG_MOUNT_POINT, nil
}

type dirJuncFileInfo struct {
	fileInfo *os.FileInfo
}

func (fi *dirJuncFileInfo) Name() string {
	return (*fi.fileInfo).Name()
}

func (fi *dirJuncFileInfo) Size() int64 {
	return (*fi.fileInfo).Size()
}

func (fi *dirJuncFileInfo) Mode() os.FileMode {
	return (*fi.fileInfo).Mode() ^ os.ModeSymlink | os.ModeDir
}

func (fi *dirJuncFileInfo) ModTime() time.Time {
	return (*fi.fileInfo).ModTime()
}

func (fi *dirJuncFileInfo) IsDir() bool {
	return true
}

func (fi *dirJuncFileInfo) Sys() interface{} {
	return (*fi.fileInfo).Sys()
}

func underlyingLstat(name string) (os.FileInfo, error) {
	var fi, err = os.Lstat(name)

	// NTFS directory junctions are treated as ordinary directories,
	// see https://forum.syncthing.net/t/option-to-follow-directory-junctions-symbolic-links/14750
	if err == nil && fi.Mode()&os.ModeSymlink != 0 {
		var isJunct bool
		isJunct, err = isDirectoryJunction(name)
		if err == nil && isJunct {
			var fi2 = &dirJuncFileInfo{
				fileInfo: &fi,
			}
			return fi2, nil
		}
	}
	return fi, err
}

// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build !windows

package fs

import (
	"io"
	"runtime"
	"syscall"
	"unsafe"
)

const FICLONERANGE = 0x4020940d

/*
http://man7.org/linux/man-pages/man2/ioctl_ficlonerange.2.html

struct file_clone_range {
   __s64 src_fd;
   __u64 src_offset;
   __u64 src_length;
   __u64 dest_offset;
};
*/
type fileCloneRange struct {
	srcFd     int64
	srcOffset uint64
	srcLength uint64
	dstOffset uint64
}

func copyRangeIoctl(src, dst fsFile, srcOffset, dstOffset, size int64) error {
	params := &fileCloneRange{
		srcFd:     int64(src.Fd()),
		srcOffset: uint64(srcOffset),
		srcLength: uint64(size),
		dstOffset: uint64(dstOffset),
	}
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, dst.Fd(), FICLONERANGE, uintptr(unsafe.Pointer(params)))
	runtime.KeepAlive(params)
	return err
}

func copyFileSendFile(src, dst fsFile, srcOffset, dstOffset, size int64) error {
	oldOffset, err := src.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	for size > 0 {
		n, err := syscall.Sendfile(int(dst.Fd()), int(src.Fd()), &dstOffset, int(size))
		if err != nil && err != syscall.EAGAIN {
			_, _ = src.Seek(oldOffset, io.SeekStart)
			return err
		}
		srcOffset += int64(n)
		dstOffset += int64(n)
		size -= int64(n)
	}
	_, err = src.Seek(oldOffset, io.SeekStart)
	return err
}

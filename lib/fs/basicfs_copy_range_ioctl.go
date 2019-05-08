// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build !windows,!solaris,!darwin

package fs

import (
	"syscall"
	"unsafe"
)

func init() {
	registerCopyRangeImplementation(copyRangeImplementation{
		name: "ioctl",
		impl: copyRangeIoctl,
	})
}

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

func copyRangeIoctl(src, dst basicFile, srcOffset, dstOffset, size int64) error {
	params := fileCloneRange{
		srcFd:     int64(src.Fd()),
		srcOffset: uint64(srcOffset),
		srcLength: uint64(size),
		dstOffset: uint64(dstOffset),
	}
	_, _, e1 := syscall.Syscall(syscall.SYS_IOCTL, dst.Fd(), FICLONERANGE, uintptr(unsafe.Pointer(&params)))
	if e1 != 0 {
		return syscall.Errno(e1)
	}
	return nil
}

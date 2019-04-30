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

func copyRangeIoctl(src, dst basicFile, srcOffset, dstOffset, size int64) error {
	params := fileCloneRange{
		srcFd:     int64(src.Fd()),
		srcOffset: uint64(srcOffset),
		srcLength: uint64(size),
		dstOffset: uint64(dstOffset),
	}
	_, _, e1 := syscall.Syscall(syscall.SYS_IOCTL, dst.Fd(), FICLONERANGE, uintptr(unsafe.Pointer(&params)))
	runtime.KeepAlive(params)
	if e1 != 0 {
		return syscall.Errno(e1)
	}
	return nil
}

func copyFileSendFile(src, dst basicFile, srcOffset, dstOffset, size int64) error {
	// Check that the destination file has sufficient space
	if fi, err := dst.Stat(); err != nil {
		return err
	} else if fi.Size() < dstOffset+size {
		if err := dst.Truncate(dstOffset + size); err != nil {
			return err
		}
	}

	// Seek to the offset we expect to write
	if n, err := dst.Seek(dstOffset, io.SeekStart); err != nil {
		return err
	} else if n != dstOffset {
		return io.ErrUnexpectedEOF
	}

	for size > 0 {
		// From the MAN page:
		//
		// If offset is not NULL, then it points to a variable holding the file offset from which sendfile() will start
		// reading data from in_fd. When sendfile() returns, this variable will be set to the offset of the byte
		// following the last byte that was read. If offset is not NULL, then sendfile() does not modify the current
		// file offset of in_fd; otherwise the current file offset is adjusted to reflect the number of bytes read from
		// in_fd.
		n, err := syscall.Sendfile(int(dst.Fd()), int(src.Fd()), &srcOffset, int(size))

		if n == 0 && err == nil {
			err = io.ErrShortWrite
		}

		if err != nil && err != syscall.EAGAIN {
			_, _ = dst.Seek(dstOffset, io.SeekStart)
			return err
		}

		size -= int64(n)
	}

	if _, err := dst.Seek(dstOffset, io.SeekStart); err != nil {
		return err
	}

	return nil
}

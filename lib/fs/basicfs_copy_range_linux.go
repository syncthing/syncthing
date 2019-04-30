// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build linux

package fs

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func copyRangeOptimised(src, dst basicFile, srcOffset, dstOffset, size int64) error {
	for _, opt := range copyOptimisations {
		switch opt {
		case "ioctl":
			if err := copyRangeIoctl(src, dst, srcOffset, dstOffset, size); err == nil {
				return nil
			}
		case "copy_file_range":
			if err := copyRangeCopyFileRange(src, dst, srcOffset, dstOffset, size); err == nil {
				return nil
			}
		case "sendfile":
			if err := copyFileSendFile(src, dst, srcOffset, dstOffset, size); err == nil {
				return nil
			}
		}
	}
	return syscall.ENOTSUP
}

func copyRangeCopyFileRange(src, dst basicFile, srcOffset, dstOffset, size int64) error {
	for size > 0 {
		// From MAN page:
		//
		// If off_in is not NULL, then off_in must point to a buffer that
		// specifies the starting offset where bytes from fd_in will be read.
		//	The file offset of fd_in is not changed, but off_in is adjusted
		// appropriately.
		n, err := unix.CopyFileRange(int(src.Fd()), &srcOffset, int(dst.Fd()), &dstOffset, int(size), 0)
		if err != nil && err != syscall.EAGAIN {
			return err
		}
		dstOffset += int64(n)
		size -= int64(n)
	}
	return nil
}

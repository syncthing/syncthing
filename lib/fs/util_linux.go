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

func copyRangeOptimised(src, dst *fsFile, srcOffset, dstOffset, size int64) error {
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

func copyRangeCopyFileRange(srcFd, dstFd *fsFile, srcOffset, dstOffset, size int64) error {
	for size > 0 {
		n, err := unix.CopyFileRange(int(srcFd.Fd()), &srcOffset, int(dstFd.Fd()), &dstOffset, int(size), 0)
		if err != nil && err != syscall.EAGAIN {
			return err
		}
		srcOffset += int64(n)
		dstOffset += int64(n)
		size -= int64(n)
	}
	return nil
}

// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build linux
// +build linux

package fs

import (
	"io"
	"syscall"

	"golang.org/x/sys/unix"
)

func init() {
	registerCopyRangeImplementation(CopyRangeMethodCopyFileRange, copyRangeImplementationForBasicFile(copyRangeCopyFileRange))
}

func copyRangeCopyFileRange(src, dst basicFile, srcOffset, dstOffset, size int64) error {
	for size > 0 {
		// From MAN page:
		//
		// If off_in is not NULL, then off_in must point to a buffer that
		// specifies the starting offset where bytes from fd_in will be read.
		// 	The file offset of fd_in is not changed, but off_in is adjusted
		// appropriately.
		//
		// Also, even if explicitly not stated, the same is true for dstOffset
		n, err := withFileDescriptors(src, dst, func(srcFd, dstFd uintptr) (int, error) {
			return unix.CopyFileRange(int(srcFd), &srcOffset, int(dstFd), &dstOffset, int(size), 0)
		})
		if n == 0 && err == nil {
			return io.ErrUnexpectedEOF
		}
		if err != nil && err != syscall.EAGAIN {
			return err
		}
		// Handle case where err == EAGAIN and n == -1 (it's not clear if that can happen)
		if n > 0 {
			size -= int64(n)
		}
	}
	return nil
}

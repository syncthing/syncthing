// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build linux || solaris
// +build linux solaris

package fs

import (
	"io"
	"syscall"
)

func init() {
	registerCopyRangeImplementation(CopyRangeMethodSendFile, copyRangeImplementationForBasicFile(copyRangeSendFile))
}

func copyRangeSendFile(src, dst basicFile, srcOffset, dstOffset, size int64) error {
	// Check that the destination file has sufficient space
	if fi, err := dst.Stat(); err != nil {
		return err
	} else if fi.Size() < dstOffset+size {
		if err := dst.Truncate(dstOffset + size); err != nil {
			return err
		}
	}

	// Record old dst offset.
	oldDstOffset, err := dst.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	defer func() { _, _ = dst.Seek(oldDstOffset, io.SeekStart) }()

	// Seek to the offset we expect to write
	if oldDstOffset != dstOffset {
		if n, err := dst.Seek(dstOffset, io.SeekStart); err != nil {
			return err
		} else if n != dstOffset {
			return io.ErrUnexpectedEOF
		}
	}

	for size > 0 {
		// From the MAN page:
		//
		// If offset is not NULL, then it points to a variable holding the file offset from which sendfile() will start
		// reading data from in_fd. When sendfile() returns, this variable will be set to the offset of the byte
		// following the last byte that was read. If offset is not NULL, then sendfile() does not modify the current
		// file offset of in_fd; otherwise the current file offset is adjusted to reflect the number of bytes read from
		// in_fd.
		n, err := withFileDescriptors(dst, src, func(dstFd, srcFd uintptr) (int, error) {
			return syscall.Sendfile(int(dstFd), int(srcFd), &srcOffset, int(size))
		})
		if n == 0 && err == nil {
			err = io.ErrUnexpectedEOF
		}
		if err != nil && err != syscall.EAGAIN {
			return err
		}
		// Handle case where err == EAGAIN and n == -1 (it's not clear if that can happen)
		if n > 0 {
			size -= int64(n)
		}
	}

	_, err = dst.Seek(oldDstOffset, io.SeekStart)
	return err
}

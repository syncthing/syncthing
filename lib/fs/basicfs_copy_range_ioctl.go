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

	"golang.org/x/sys/unix"
)

func init() {
	registerCopyRangeImplementation(CopyRangeMethodIoctl, copyRangeImplementationForBasicFile(copyRangeIoctl))
}

func copyRangeIoctl(src, dst basicFile, srcOffset, dstOffset, size int64) error {
	fi, err := src.Stat()
	if err != nil {
		return err
	}

	if srcOffset+size > fi.Size() {
		return io.ErrUnexpectedEOF
	}

	// https://www.man7.org/linux/man-pages/man2/ioctl_ficlonerange.2.html
	// If src_length is zero, the ioctl reflinks to the end of the source file.
	if srcOffset+size == fi.Size() {
		size = 0
	}

	if srcOffset == 0 && dstOffset == 0 && size == 0 {
		// Optimization for whole file copies.
		_, err := withFileDescriptors(src, dst, func(srcFd, dstFd uintptr) (int, error) {
			return 0, unix.IoctlFileClone(int(dstFd), int(srcFd))
		})
		return err
	}

	_, err = withFileDescriptors(src, dst, func(srcFd, dstFd uintptr) (int, error) {
		params := unix.FileCloneRange{
			Src_fd:      int64(srcFd),
			Src_offset:  uint64(srcOffset),
			Src_length:  uint64(size),
			Dest_offset: uint64(dstOffset),
		}
		return 0, unix.IoctlFileCloneRange(int(dstFd), &params)
	})
	return err
}

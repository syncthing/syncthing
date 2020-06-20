// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build windows

package fs

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func init() {
	registerCopyRangeImplementation(CopyRangeMethodDuplicateExtents, copyRangeImplementationForBasicFile(copyRangeDuplicateExtents))
}

// Inspired by https://github.com/git-lfs/git-lfs/blob/master/tools/util_windows.go

var (
	availableClusterSize = []int64{64 * 1024, 4 * 1024} // ReFS only supports 64KiB and 4KiB cluster.
	GiB                  = int64(1024 * 1024 * 1024)
)

// fsctlDuplicateExtentsToFile = FSCTL_DUPLICATE_EXTENTS_TO_FILE IOCTL
// Instructs the file system to copy a range of file bytes on behalf of an application.
//
// https://docs.microsoft.com/windows/win32/api/winioctl/ni-winioctl-fsctl_duplicate_extents_to_file
const fsctlDuplicateExtentsToFile = 623428

// duplicateExtentsData = DUPLICATE_EXTENTS_DATA structure
// Contains parameters for the FSCTL_DUPLICATE_EXTENTS control code that performs the Block Cloning operation.
//
// https://docs.microsoft.com/windows/win32/api/winioctl/ns-winioctl-duplicate_extents_data
type duplicateExtentsData struct {
	FileHandle       windows.Handle
	SourceFileOffset int64
	TargetFileOffset int64
	ByteCount        int64
}

func copyRangeDuplicateExtents(src, dst basicFile, srcOffset, dstOffset, size int64) error {
	var err error
	// Check that the destination file has sufficient space
	if fi, err := dst.Stat(); err != nil {
		return err
	} else if fi.Size() < dstOffset+size {
		// set file size. There is a requirements "The destination region must not extend past the end of file."
		if err = dst.Truncate(dstOffset + size); err != nil {
			return err
		}
	}

	// Requirement
	// * The source and destination regions must begin and end at a cluster boundary. (4KiB or 64KiB)
	// * cloneRegionSize less than 4GiB.
	// see https://docs.microsoft.com/windows/win32/fileio/block-cloning

	// Clone first xGiB region.
	for size > GiB {
		err = callDuplicateExtentsToFile(src.Fd(), dst.Fd(), srcOffset, dstOffset, GiB)
		if err != nil {
			return wrapError(err)
		}
		size -= GiB
		srcOffset += GiB
		dstOffset += GiB
	}

	// Clone tail. First try with 64KiB round up, then fallback to 4KiB.
	for _, cloneRegionSize := range availableClusterSize {
		err = callDuplicateExtentsToFile(src.Fd(), dst.Fd(), srcOffset, dstOffset, roundUp(size, cloneRegionSize))
		if err != nil {
			continue
		}
		break
	}

	return wrapError(err)
}

func wrapError(err error) error {
	if err == windows.SEVERITY_ERROR {
		return syscall.ENOTSUP
	}
	return err
}

// call FSCTL_DUPLICATE_EXTENTS_TO_FILE IOCTL
// see https://docs.microsoft.com/en-us/windows/win32/api/winioctl/ni-winioctl-fsctl_duplicate_extents_to_file
//
// memo: Overflow (cloneRegionSize is greater than file ends) is safe and just ignored by windows.
func callDuplicateExtentsToFile(src, dst uintptr, srcOffset, dstOffset int64, cloneRegionSize int64) (err error) {
	var (
		bytesReturned uint32
		overlapped    windows.Overlapped
	)

	request := duplicateExtentsData{
		FileHandle:       windows.Handle(src),
		SourceFileOffset: srcOffset,
		TargetFileOffset: dstOffset,
		ByteCount:        cloneRegionSize,
	}

	return windows.DeviceIoControl(
		windows.Handle(dst),
		fsctlDuplicateExtentsToFile,
		(*byte)(unsafe.Pointer(&request)),
		uint32(unsafe.Sizeof(request)),
		(*byte)(unsafe.Pointer(nil)), // = nullptr
		0,
		&bytesReturned,
		&overlapped)
}

func roundUp(value, base int64) int64 {
	mod := value % base
	if mod == 0 {
		return value
	}

	return value - mod + base
}

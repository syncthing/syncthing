// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build windows

package fs

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// FSCTL control codes, see learn.microsoft.com "Sparse Files".
const (
	fsctlSetSparse            = 0x000900C4 // FSCTL_SET_SPARSE
	fsctlQueryAllocatedRanges = 0x000940CF // FSCTL_QUERY_ALLOCATED_RANGES
)

type fileAllocatedRangeBuffer struct {
	FileOffset int64
	Length     int64
}

// SetSparse marks the file sparse via FSCTL_SET_SPARSE; NTFS won't make a
// Truncate'd file sparse on its own. Best effort: filesystems without sparse
// support (FAT, exFAT, some network mounts) are left as-is, and NextHole then
// returns false.
func SetSparse(f File) {
	bf, ok := unwrap(f).(basicFile)
	if !ok {
		return
	}
	var ret uint32
	_ = windows.DeviceIoControl(windows.Handle(bf.Fd()), fsctlSetSparse, nil, 0, nil, 0, &ret, nil)
}

// NextHole returns the next hole at or after offset via
// FSCTL_QUERY_ALLOCATED_RANGES (the Windows SEEK_HOLE). It first checks the file
// is actually sparse: a non-sparse file reports every range allocated, so it
// returns false rather than let the caller infer presence. EOF counts as a hole.
func NextHole(f File, offset int64) (int64, bool) {
	bf, ok := unwrap(f).(basicFile)
	if !ok {
		return 0, false
	}
	handle := windows.Handle(bf.Fd())

	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return 0, false
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_SPARSE_FILE == 0 {
		return 0, false // not sparse; see doc comment
	}
	size := int64(info.FileSizeHigh)<<32 | int64(info.FileSizeLow)
	if offset >= size {
		return offset, true // at or beyond EOF is a hole
	}

	in := fileAllocatedRangeBuffer{FileOffset: offset, Length: size - offset}
	out := make([]fileAllocatedRangeBuffer, 64)
	var ret uint32
	err := windows.DeviceIoControl(handle, fsctlQueryAllocatedRanges,
		(*byte)(unsafe.Pointer(&in)), uint32(unsafe.Sizeof(in)),
		(*byte)(unsafe.Pointer(&out[0])), uint32(len(out))*uint32(unsafe.Sizeof(out[0])),
		&ret, nil)
	// ERROR_MORE_DATA only means there are more ranges than fit the buffer; the
	// first returned range is still valid and is all we need.
	if err != nil && err != windows.ERROR_MORE_DATA {
		return 0, false
	}
	n := int(ret) / int(unsafe.Sizeof(out[0]))
	if n == 0 || out[0].FileOffset > offset {
		return offset, true // hole at offset
	}
	// Allocated region covers offset; the next hole is at its end.
	return out[0].FileOffset + out[0].Length, true
}

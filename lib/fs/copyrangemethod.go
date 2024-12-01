// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

type CopyRangeMethod int32

const (
	CopyRangeMethodStandard         CopyRangeMethod = 0
	CopyRangeMethodIoctl            CopyRangeMethod = 1
	CopyRangeMethodCopyFileRange    CopyRangeMethod = 2
	CopyRangeMethodSendFile         CopyRangeMethod = 3
	CopyRangeMethodDuplicateExtents CopyRangeMethod = 4
	CopyRangeMethodAllWithFallback  CopyRangeMethod = 5
)

func (o CopyRangeMethod) String() string {
	switch o {
	case CopyRangeMethodStandard:
		return "standard"
	case CopyRangeMethodIoctl:
		return "ioctl"
	case CopyRangeMethodCopyFileRange:
		return "copy_file_range"
	case CopyRangeMethodSendFile:
		return "sendfile"
	case CopyRangeMethodDuplicateExtents:
		return "duplicate_extents"
	case CopyRangeMethodAllWithFallback:
		return "all"
	default:
		return "unknown"
	}
}

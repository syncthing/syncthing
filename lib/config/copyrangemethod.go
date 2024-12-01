// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import "github.com/syncthing/syncthing/lib/fs"

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

func (o CopyRangeMethod) ToFS() fs.CopyRangeMethod {
	switch o {
	case CopyRangeMethodStandard:
		return fs.CopyRangeMethodStandard
	case CopyRangeMethodIoctl:
		return fs.CopyRangeMethodIoctl
	case CopyRangeMethodCopyFileRange:
		return fs.CopyRangeMethodCopyFileRange
	case CopyRangeMethodSendFile:
		return fs.CopyRangeMethodSendFile
	case CopyRangeMethodDuplicateExtents:
		return fs.CopyRangeMethodDuplicateExtents
	case CopyRangeMethodAllWithFallback:
		return fs.CopyRangeMethodAllWithFallback
	default:
		return fs.CopyRangeMethodStandard
	}
}

func (o CopyRangeMethod) MarshalText() ([]byte, error) {
	return []byte(o.String()), nil
}

func (o *CopyRangeMethod) UnmarshalText(bs []byte) error {
	switch string(bs) {
	case "standard":
		*o = CopyRangeMethodStandard
	case "ioctl":
		*o = CopyRangeMethodIoctl
	case "copy_file_range":
		*o = CopyRangeMethodCopyFileRange
	case "sendfile":
		*o = CopyRangeMethodSendFile
	case "duplicate_extents":
		*o = CopyRangeMethodDuplicateExtents
	case "all":
		*o = CopyRangeMethodAllWithFallback
	default:
		*o = CopyRangeMethodStandard
	}
	return nil
}

func (o *CopyRangeMethod) ParseDefault(str string) error {
	return o.UnmarshalText([]byte(str))
}

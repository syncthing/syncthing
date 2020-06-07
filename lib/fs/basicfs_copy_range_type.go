// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

type CopyRangeType int

const (
	CopyRangeTypeStandard CopyRangeType = iota
	CopyRangeTypeIoctl
	CopyRangeTypeCopyFileRange
	CopyRangeTypeSendFile
	CopyRangeTypeAllWithFallback
)

func (o CopyRangeType) String() string {
	switch o {
	case CopyRangeTypeStandard:
		return "standard"
	case CopyRangeTypeIoctl:
		return "ioctl"
	case CopyRangeTypeCopyFileRange:
		return "copy_file_range"
	case CopyRangeTypeSendFile:
		return "sendfile"
	case CopyRangeTypeAllWithFallback:
		return "all"
	default:
		return "unknown"
	}
}

func (o CopyRangeType) MarshalText() ([]byte, error) {
	return []byte(o.String()), nil
}

func (o *CopyRangeType) UnmarshalText(bs []byte) error {
	switch string(bs) {
	case "standard":
		*o = CopyRangeTypeStandard
	case "ioctl":
		*o = CopyRangeTypeIoctl
	case "copy_file_range":
		*o = CopyRangeTypeCopyFileRange
	case "sendfile":
		*o = CopyRangeTypeSendFile
	case "all":
		*o = CopyRangeTypeAllWithFallback
	default:
		*o = CopyRangeTypeStandard
	}
	return nil
}

// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

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

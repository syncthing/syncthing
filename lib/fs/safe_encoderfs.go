// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

type SafeEncoderFilesystem struct {
	EncoderFilesystem
}

var safeReservedChars = string([]rune{
	// 0x00 is disallowed but we should never see it in a filename
	0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
	0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
	0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
	0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
	//'"' '*'   ':'   '<'   '>'   '?'   '|'   DEL
	0x22, 0x2a, 0x3a, 0x3c, 0x3e, 0x3f, 0x7c, 0x7f,
	// 0x2f (/) is disallowed, but we never see it in a filename
	// 0x5c (\) is disallowed, but we never see it in a filename
	0x80, 0x81, 0x82, 0x83, 0x84, 0x85, 0x86, 0x87,
	0x88, 0x89, 0x8a, 0x8b, 0x8c, 0x8d, 0x8e, 0x8f,
	0x90, 0x91, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97,
	0x98, 0x99, 0x9a, 0x9b, 0x9c, 0x9d, 0x9e, 0x9f,
})

const safeReservedStartChars = windowsReservedStartChars
const safeReservedEndChars = windowsReservedEndChars

// const safeReservedNames = windowsDisallowedNames

// A NewSafeEncoderFilesystem ensures that paths that contain characters
// that are reserved in several filesystems can be safety stored.
func NewSafeEncoderFilesystem(fs Filesystem) Filesystem {
	return wrapFilesystem(fs, func(underlying Filesystem) Filesystem {
		efs := EncoderFilesystem{
			Filesystem:         underlying,
			reservedChars:      safeReservedChars,
			reservedStartChars: safeReservedStartChars,
			reservedEndChars:   safeReservedEndChars,
			// reservedNames:      safeReservedNames,
		}
		efs.init()
		return &SafeEncoderFilesystem{efs}
	})
}

func newSafeEncoderFilesystem(fs Filesystem) *SafeEncoderFilesystem {
	return NewSafeEncoderFilesystem(fs).(*SafeEncoderFilesystem)
}

func (f *SafeEncoderFilesystem) EncoderType() FilesystemEncoderType {
	return FilesystemEncoderTypeSafe
}

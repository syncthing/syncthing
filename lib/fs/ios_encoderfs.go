// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

type IosEncoderFilesystem struct {
	EncoderFilesystem
}

var iosReservedChars = string([]rune{
	0x00,
	// 0x2f "/" is disallowed, but we never see it in a filename
	0x3a, // ":"
})

const iosReservedStartChars = "."
const iosReservedEndChars = ""

// A NewIosEncoderFilesystem ensures that paths that contain characters
// that are reserved in the iOS filesystem can be safety stored.
func NewIosEncoderFilesystem(fs Filesystem) Filesystem {
	return wrapFilesystem(fs, func(underlying Filesystem) Filesystem {
		efs := EncoderFilesystem{
			Filesystem:         underlying,
			reservedChars:      iosReservedChars,
			reservedStartChars: iosReservedStartChars,
			reservedEndChars:   iosReservedEndChars,
		}
		efs.init()
		return &IosEncoderFilesystem{efs}
	})
}

/* Not currently used:
func newIosEncoderFilesystem(fs Filesystem) *IosEncoderFilesystem {
	return NewIosEncoderFilesystem(fs).(*IosEncoderFilesystem)
}
*/

func (f *IosEncoderFilesystem) EncoderType() FilesystemEncoderType {
	return FilesystemEncoderTypeIos
}

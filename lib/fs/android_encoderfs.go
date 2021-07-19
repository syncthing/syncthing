// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

type AndroidEncoderFilesystem struct {
	EncoderFilesystem
}

var androidReservedChars = string([]rune{
	// 0x00 is disallowed but we should never see it in a filename
	0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
	0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
	0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
	0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
	// 0x2f (/) is disallowed, but we never see it in a filename
})

const androidReservedStartChars = ""
const androidReservedEndChars = ""

// A NewAndroidEncoderFilesystem ensures that paths that contain characters
// that are reserved in the Android fuse filesystems (<>:"|?*) can be safety
// stored.
func NewAndroidEncoderFilesystem(fs Filesystem) Filesystem {
	return wrapFilesystem(fs, func(underlying Filesystem) Filesystem {
		efs := EncoderFilesystem{
			Filesystem:         underlying,
			reservedChars:      androidReservedChars,
			reservedStartChars: androidReservedStartChars,
			reservedEndChars:   androidReservedEndChars,
		}
		efs.init()
		return &AndroidEncoderFilesystem{efs}
	})
}

func newAndroidEncoderFilesystem(fs Filesystem) *AndroidEncoderFilesystem {
	return NewAndroidEncoderFilesystem(fs).(*AndroidEncoderFilesystem)
}

func (f *AndroidEncoderFilesystem) EncoderType() FilesystemEncoderType {
	return FilesystemEncoderTypeAndroid
}

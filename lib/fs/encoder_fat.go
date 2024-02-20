// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

// There are few, if any, filesystems that allow NULs in filenames, so
// encoding it is not necessary.
var fatCharsToEncode = string([]rune{
	/* */ 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
	0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
	0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
	0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
	/**/ '"', '*', ':', '<', '>', '?', '|',
	//   0x22,0x2a,0x3a,0x3c,0x3e,0x3f,0x7c,
	// The slash character (0x2f) is never encoded
	// The backslash character (0x5c) is never encoded
})

func init() {
	encoder := &baseEncoder{charsToEncode: fatCharsToEncode}
	// Register encoder, and add it to the test matrix
	registerEncoder(FilesystemEncoderTypeFat, encoder, new(OptionFatEncoder))
}

type OptionFatEncoder struct{}

func (*OptionFatEncoder) apply(fs Filesystem) Filesystem {
	if basic, ok := fs.(*BasicFilesystem); !ok {
		l.Warnln("OptionFatEncoder must only be used with FilesystemTypeBasic")
	} else {
		basic.encoderType = FilesystemEncoderTypeFat
	}
	return fs
}

func (*OptionFatEncoder) String() string {
	return "fatEncoder"
}

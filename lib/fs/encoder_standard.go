// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

func init() {
	// We can't assume that the underlying file system has unicode
	// capabilities, so the standard encoder doesn't encode any
	// characters. Also most unix-like filesystems, such as ext4, allow
	// all characters in a file or directory name except for slash (/)
	// and NUL (\x00). There are few, if any, filesystems that allow NULs
	// in filenames, so encoding it is not necessary.
	var encoder = &baseEncoder{charsToEncode: ""}
	// Register encoder, and add it to the test matrix.
	registerEncoder(FilesystemEncoderTypeStandard, encoder, new(OptionStandardEncoder))
}

type OptionStandardEncoder struct{}

func (*OptionStandardEncoder) apply(fs Filesystem) Filesystem {
	if basic, ok := fs.(*BasicFilesystem); !ok {
		l.Warnln("OptionStandardEncoder must only be used with FilesystemTypeBasic")
	} else {
		basic.encoderType = FilesystemEncoderTypeStandard
	}
	return fs
}

func (*OptionStandardEncoder) String() string {
	return "standardEncoder"
}

// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

func init() {
	// A passthru encoder is represented by nil, as it doesn't encode/decode
	var passthroughEncoder encoder = nil
	// Register encoder, and add it to the test matrix
	registerEncoder(FilesystemEncoderTypePassthrough, passthroughEncoder, new(OptionPassthroughEncoder))
}

type OptionPassthroughEncoder struct{}

func (*OptionPassthroughEncoder) apply(fs Filesystem) Filesystem {
	if basic, ok := fs.(*BasicFilesystem); !ok {
		l.Warnln("OptionPassthroughEncoder must only be used with FilesystemTypeBasic")
	} else {
		basic.encoderType = FilesystemEncoderTypePassthrough
	}
	return fs
}

func (*OptionPassthroughEncoder) String() string {
	return "passthroughEncoder"
}

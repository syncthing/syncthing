// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build dragonfly || illumos || solaris || openbsd
// +build dragonfly illumos solaris openbsd

package fs

import (
	"github.com/syncthing/syncthing/lib/protocol"
)

func (f *BasicFilesystem) GetXattr(path string, xattrFilter StringFilter) ([]protocol.Xattr, error) {
	return nil, syscall.ENOTSUP
}

func (f *BasicFilesystem) SetXattr(path string, xattrs []protocol.Xattr, xattrFilter StringFilter) error {
	return syscall.ENOTSUP
}

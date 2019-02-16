// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build windows

package fs

import (
	"syscall"
)

func copyRangeOptimised(src, dst *fsFile, srcOffset, dstOffset, size int64) error {
	return syscall.ENOTSUP
}

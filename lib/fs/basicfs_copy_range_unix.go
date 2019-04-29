// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build !linux,!windows

package fs

import (
	"syscall"
)

func copyRangeOptimised(src, dst fsFile, srcOffset, dstOffset, size int64) error {
	for _, opt := range copyOptimisations {
		switch opt {
		case "ioctl":
			if err := copyRangeIoctl(src, dst, srcOffset, dstOffset, size); err == nil {
				return nil
			}
		case "sendfile":
			if err := copyFileSendFile(src, dst, srcOffset, dstOffset, size); err == nil {
				return nil
			}
		}
	}
	return syscall.ENOTSUP
}

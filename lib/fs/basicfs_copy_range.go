// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"syscall"
)

type copyRangeImplementationBasicFile func(src, dst basicFile, srcOffset, dstOffset, size int64) error

func copyRangeImplementationForBasicFile(impl copyRangeImplementationBasicFile) copyRangeImplementation {
	return func(src, dst File, srcOffset, dstOffset, size int64) error {
		srcFile, srcOk := src.(basicFile)
		dstFile, dstOk := dst.(basicFile)
		if !srcOk || !dstOk {
			return syscall.ENOTSUP
		}
		return impl(srcFile, dstFile, srcOffset, dstOffset, size)
	}
}

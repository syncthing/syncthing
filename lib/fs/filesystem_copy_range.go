// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"sync"
	"syscall"
)

var (
	copyRangeMethods = make(map[CopyRangeMethod]copyRangeImplementation)
	mut              sync.Mutex
)

type copyRangeImplementation func(src, dst File, srcOffset, dstOffset, size int64) error

func registerCopyRangeImplementation(copyMethod CopyRangeMethod, impl copyRangeImplementation) {
	mut.Lock()
	defer mut.Unlock()

	l.Debugln("Registering " + copyMethod.String() + " copyRange method")

	copyRangeMethods[copyMethod] = impl
}

// CopyRange tries to use the specified method to copy data between two files.
// Takes size bytes at offset srcOffset from the source file, and copies the data to destination file at offset
// dstOffset. If required, adjusts the size of the destination file to fit that much data.
//
// On Linux/BSD you can ask it to use ioctl and copy_file_range system calls, which if the underlying filesystem supports
// it tries referencing existing data in the source file, instead of making a copy and taking up additional space.
//
// CopyRange does its best to have no effect on src and dst file offsets (copy operation should not affect it).
func CopyRange(copyMethod CopyRangeMethod, src, dst File, srcOffset, dstOffset, size int64) error {
	if impl, ok := copyRangeMethods[copyMethod]; ok {
		return impl(src, dst, srcOffset, dstOffset, size)
	}

	return syscall.ENOTSUP
}

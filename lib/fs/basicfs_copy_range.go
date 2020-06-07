// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"syscall"

	"github.com/syncthing/syncthing/lib/sync"
)

var (
	copyRangeImplementations = make(map[CopyRangeType]copyRangeImplementation)
	mut                      = sync.NewMutex()
)

type copyRangeImplementation func(src, dst basicFile, srcOffset, dstOffset, size int64) error

func registerCopyRangeImplementation(copyType CopyRangeType, impl copyRangeImplementation) {
	mut.Lock()
	defer mut.Unlock()

	l.Debugln("Registering " + copyType.String() + " copyRange implementation")

	copyRangeImplementations[copyType] = impl
}

// CopyRange tries to use the most optimal way to copy data between two files.
// Takes size bytes at offset srcOffset from the source file, and copies the data to destination file at offset
// dstOffset. If required, adjusts the size of the destination file to fit that much data.
//
// On Linux/BSD it tries to use ioctl and copy_file_range system calls, which if the underlying filesystem supports it
// tries referencing existing data in the source file, instead of making a copy and taking up additional space.
//
// If that is not possible, the data will be copied using an in-kernel copy (copy_file_range fallback, sendfile),
// oppose to user space copy, if those system calls are available and supported for the source and target in question.
//
// CopyRange does it's best to have no effect on src and dst file offsets (copy operation should not affect it).
func CopyRange(copyType CopyRangeType, src, dst File, srcOffset, dstOffset, size int64) error {
	srcFile, srcOk := src.(basicFile)
	dstFile, dstOk := dst.(basicFile)
	if !srcOk || !dstOk {
		return syscall.ENOTSUP
	}

	if impl, ok := copyRangeImplementations[copyType]; !ok {
		return syscall.ENOTSUP
	} else {
		return impl(srcFile, dstFile, srcOffset, dstOffset, size)
	}
}

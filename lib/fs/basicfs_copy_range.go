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
		src = unwrap(src)
		dst = unwrap(dst)
		// Then see if it's basic files
		srcFile, srcOk := src.(basicFile)
		dstFile, dstOk := dst.(basicFile)
		if !srcOk || !dstOk {
			return syscall.ENOTSUP
		}
		return impl(srcFile, dstFile, srcOffset, dstOffset, size)
	}
}

func withFileDescriptors(first, second basicFile, fn func(first, second uintptr) (int, error)) (int, error) {
	fc, err := first.SyscallConn()
	if err != nil {
		return 0, err
	}
	sc, err := second.SyscallConn()
	if err != nil {
		return 0, err
	}
	var n int
	var ferr, serr, fnerr error
	ferr = fc.Control(func(first uintptr) {
		serr = sc.Control(func(second uintptr) {
			n, fnerr = fn(first, second)
		})
	})
	if ferr != nil {
		return n, ferr
	}
	if serr != nil {
		return n, serr
	}
	return n, fnerr
}

func unwrap(f File) File {
	for {
		if wrapped, ok := f.(interface{ unwrap() File }); ok {
			f = wrapped.unwrap()
		} else {
			return f
		}
	}
}

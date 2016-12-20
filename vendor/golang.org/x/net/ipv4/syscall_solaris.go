// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv4

import (
	"syscall"
	"unsafe"
)

//go:cgo_import_dynamic libc___xnet_getsockopt __xnet_getsockopt "libsocket.so"
//go:cgo_import_dynamic libc_setsockopt setsockopt "libsocket.so"

//go:linkname procGetsockopt libc___xnet_getsockopt
//go:linkname procSetsockopt libc_setsockopt

var (
	procGetsockopt uintptr
	procSetsockopt uintptr
)

func sysvicall6(trap, nargs, a1, a2, a3, a4, a5, a6 uintptr) (uintptr, uintptr, syscall.Errno)

func getsockopt(s uintptr, level, name int, v unsafe.Pointer, l *uint32) error {
	_, _, errno := sysvicall6(uintptr(unsafe.Pointer(&procGetsockopt)), 5, s, uintptr(level), uintptr(name), uintptr(v), uintptr(unsafe.Pointer(l)), 0)
	if errno != 0 {
		return error(errno)
	}
	return nil
}

func setsockopt(s uintptr, level, name int, v unsafe.Pointer, l uint32) error {
	if _, _, errno := sysvicall6(uintptr(unsafe.Pointer(&procSetsockopt)), 5, s, uintptr(level), uintptr(name), uintptr(v), uintptr(l), 0); errno != 0 {
		return error(errno)
	}
	return nil
}

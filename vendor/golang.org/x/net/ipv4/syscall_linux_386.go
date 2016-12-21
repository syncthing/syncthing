// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv4

import (
	"syscall"
	"unsafe"
)

const (
	sysGETSOCKOPT = 0xf
	sysSETSOCKOPT = 0xe
)

func socketcall(call int, a0, a1, a2, a3, a4, a5 uintptr) (int, syscall.Errno)

func getsockopt(s uintptr, level, name int, v unsafe.Pointer, l *uint32) error {
	if _, errno := socketcall(sysGETSOCKOPT, s, uintptr(level), uintptr(name), uintptr(v), uintptr(unsafe.Pointer(l)), 0); errno != 0 {
		return error(errno)
	}
	return nil
}

func setsockopt(s uintptr, level, name int, v unsafe.Pointer, l uint32) error {
	if _, errno := socketcall(sysSETSOCKOPT, s, uintptr(level), uintptr(name), uintptr(v), uintptr(l), 0); errno != 0 {
		return error(errno)
	}
	return nil
}

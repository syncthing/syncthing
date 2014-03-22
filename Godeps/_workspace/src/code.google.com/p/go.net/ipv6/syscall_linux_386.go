// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This code is a duplicate of syscall/syscall_linux_386.go with small
// modifications.

package ipv6

import (
	"syscall"
	"unsafe"
)

// On x86 Linux, all the socket calls go through an extra indirection,
// I think because the 5-register system call interface can't handle
// the 6-argument calls like sendto and recvfrom. Instead the
// arguments to the underlying system call are the number below and a
// pointer to an array of uintptr. We hide the pointer in the
// socketcall assembly to avoid allocation on every system call.

const (
	// See /usr/include/linux/net.h.
	_SETSOCKOPT = 14
	_GETSOCKOPT = 15
)

var socketcall func(call int, a0, a1, a2, a3, a4, a5 uintptr) (int, syscall.Errno)

func getsockopt(fd int, level int, name int, v uintptr, l *sysSockoptLen) error {
	if _, errno := socketcall(_GETSOCKOPT, uintptr(fd), uintptr(level), uintptr(name), uintptr(v), uintptr(unsafe.Pointer(l)), 0); errno != 0 {
		return error(errno)
	}
	return nil
}

func setsockopt(fd int, level int, name int, v uintptr, l uintptr) error {
	if _, errno := socketcall(_SETSOCKOPT, uintptr(fd), uintptr(level), uintptr(name), v, l, 0); errno != 0 {
		return error(errno)
	}
	return nil
}

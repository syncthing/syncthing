// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

import (
	"syscall"
	"unsafe"
)

func getsockopt(s uintptr, level, name int, v unsafe.Pointer, l *uint32) error {
	return syscall.Getsockopt(syscall.Handle(s), int32(level), int32(name), (*byte)(v), (*int32)(unsafe.Pointer(l)))
}

func setsockopt(s uintptr, level, name int, v unsafe.Pointer, l uint32) error {
	return syscall.Setsockopt(syscall.Handle(s), int32(level), int32(name), (*byte)(v), int32(l))
}

// Copyright 2011 Evan Shaw. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin dragonfly freebsd linux openbsd solaris netbsd

package mmap

import (
	"syscall"
)

func mmap(len int, inprot, inflags, fd uintptr, off int64) ([]byte, error) {
	flags := syscall.MAP_SHARED
	prot := syscall.PROT_READ
	switch {
	case inprot&COPY != 0:
		prot |= syscall.PROT_WRITE
		flags = syscall.MAP_PRIVATE
	case inprot&RDWR != 0:
		prot |= syscall.PROT_WRITE
	}
	if inprot&EXEC != 0 {
		prot |= syscall.PROT_EXEC
	}
	if inflags&ANON != 0 {
		flags |= syscall.MAP_ANON
	}

	b, err := syscall.Mmap(int(fd), off, len, prot, flags)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func flush(addr, len uintptr) error {
	_, _, errno := syscall.Syscall(_SYS_MSYNC, addr, len, _MS_SYNC)
	if errno != 0 {
		return syscall.Errno(errno)
	}
	return nil
}

func lock(addr, len uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_MLOCK, addr, len, 0)
	if errno != 0 {
		return syscall.Errno(errno)
	}
	return nil
}

func unlock(addr, len uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_MUNLOCK, addr, len, 0)
	if errno != 0 {
		return syscall.Errno(errno)
	}
	return nil
}

func unmap(addr, len uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_MUNMAP, addr, len, 0)
	if errno != 0 {
		return syscall.Errno(errno)
	}
	return nil
}

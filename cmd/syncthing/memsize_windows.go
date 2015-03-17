// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"encoding/binary"
	"syscall"
	"unsafe"
)

var (
	kernel32, _             = syscall.LoadLibrary("kernel32.dll")
	globalMemoryStatusEx, _ = syscall.GetProcAddress(kernel32, "GlobalMemoryStatusEx")
)

func memorySize() (int64, error) {
	var memoryStatusEx [64]byte
	binary.LittleEndian.PutUint32(memoryStatusEx[:], 64)
	p := uintptr(unsafe.Pointer(&memoryStatusEx[0]))

	ret, _, callErr := syscall.Syscall(uintptr(globalMemoryStatusEx), 1, p, 0, 0)
	if ret == 0 {
		return 0, callErr
	}

	return int64(binary.LittleEndian.Uint64(memoryStatusEx[8:])), nil
}

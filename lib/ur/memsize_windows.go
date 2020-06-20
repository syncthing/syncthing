// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package ur

import (
	"encoding/binary"
	"syscall"
	"unsafe"
)

var (
	kernel32, _             = syscall.LoadLibrary("kernel32.dll")
	globalMemoryStatusEx, _ = syscall.GetProcAddress(kernel32, "GlobalMemoryStatusEx")
)

func memorySize() int64 {
	var memoryStatusEx [64]byte
	binary.LittleEndian.PutUint32(memoryStatusEx[:], 64)

	ret, _, _ := syscall.Syscall(uintptr(globalMemoryStatusEx), 1, uintptr(unsafe.Pointer(&memoryStatusEx[0])), 0, 0)
	if ret == 0 {
		return 0
	}

	return int64(binary.LittleEndian.Uint64(memoryStatusEx[8:]))
}

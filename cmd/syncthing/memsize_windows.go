// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

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

func memorySize() (uint64, error) {
	var memoryStatusEx [64]byte
	binary.LittleEndian.PutUint32(memoryStatusEx[:], 64)
	p := uintptr(unsafe.Pointer(&memoryStatusEx[0]))

	ret, _, callErr := syscall.Syscall(uintptr(globalMemoryStatusEx), 1, p, 0, 0)
	if ret == 0 {
		return 0, callErr
	}

	return binary.LittleEndian.Uint64(memoryStatusEx[8:]), nil
}

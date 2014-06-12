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

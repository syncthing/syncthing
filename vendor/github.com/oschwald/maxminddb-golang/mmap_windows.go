// +build windows,!appengine

package maxminddb

// Windows support largely borrowed from mmap-go.
//
// Copyright 2011 Evan Shaw. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

import (
	"errors"
	"os"
	"reflect"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

type memoryMap []byte

// Windows
var handleLock sync.Mutex
var handleMap = map[uintptr]windows.Handle{}

func mmap(fd int, length int) (data []byte, err error) {
	h, errno := windows.CreateFileMapping(windows.Handle(fd), nil,
		uint32(windows.PAGE_READONLY), 0, uint32(length), nil)
	if h == 0 {
		return nil, os.NewSyscallError("CreateFileMapping", errno)
	}

	addr, errno := windows.MapViewOfFile(h, uint32(windows.FILE_MAP_READ), 0,
		0, uintptr(length))
	if addr == 0 {
		return nil, os.NewSyscallError("MapViewOfFile", errno)
	}
	handleLock.Lock()
	handleMap[addr] = h
	handleLock.Unlock()

	m := memoryMap{}
	dh := m.header()
	dh.Data = addr
	dh.Len = length
	dh.Cap = dh.Len

	return m, nil
}

func (m *memoryMap) header() *reflect.SliceHeader {
	return (*reflect.SliceHeader)(unsafe.Pointer(m))
}

func flush(addr, len uintptr) error {
	errno := windows.FlushViewOfFile(addr, len)
	return os.NewSyscallError("FlushViewOfFile", errno)
}

func munmap(b []byte) (err error) {
	m := memoryMap(b)
	dh := m.header()

	addr := dh.Data
	length := uintptr(dh.Len)

	flush(addr, length)
	err = windows.UnmapViewOfFile(addr)
	if err != nil {
		return err
	}

	handleLock.Lock()
	defer handleLock.Unlock()
	handle, ok := handleMap[addr]
	if !ok {
		// should be impossible; we would've errored above
		return errors.New("unknown base address")
	}
	delete(handleMap, addr)

	e := windows.CloseHandle(windows.Handle(handle))
	return os.NewSyscallError("CloseHandle", e)
}

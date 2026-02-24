// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build windows
// +build windows

package osutil

import (
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

var (
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procAllocConsole  = kernel32.NewProc("AllocConsole")
	procAttachConsole = kernel32.NewProc("AttachConsole")
	// There is also FreeConsole, but we don't need it (Windows will clean up after termination anyway)
)

const (
	// Windows API constants
	ATTACH_PARENT_PROCESS = 0xFFFFFFFF
)

// AttachConsole attached the process to an existing console
func AttachConsole() error {

	// (ret != 0 = success, ret == 0 = failure)
	ret, _, err := procAttachConsole.Call(uintptr(ATTACH_PARENT_PROCESS))
	if ret != 0 {
		return setupConsoleHandles()
	} else {
		return err
	}
}

// InitConsole initializes a new console.
func InitConsole() error {

	// (ret != 0 = success, ret == 0 = failure)
	ret, _, err := procAllocConsole.Call()
	if ret != 0 {
		return setupConsoleHandles()
	}

	return err
}

// setupConsoleHandles referes handles to the console prepared from AttachConsole() or InitConsole()
func setupConsoleHandles() error {

	// Create file handles for console output
	conout := createConsoleFile("CONOUT$", windows.GENERIC_WRITE|windows.GENERIC_READ)
	if conout != syscall.InvalidHandle {
		err := windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(conout))
		if err != nil {
			return err
		}
		os.Stdout = os.NewFile(uintptr(conout), "CONOUT$")
	}

	// Create separate handle for stderr
	conerr := createConsoleFile("CONOUT$", windows.GENERIC_WRITE|windows.GENERIC_READ)
	if conerr != syscall.InvalidHandle {
		err := windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(conerr))
		if err != nil {
			return err
		}
		os.Stderr = os.NewFile(uintptr(conerr), "CONOUT$")
	}

	// Create handle for console input
	conin := createConsoleFile("CONIN$", windows.GENERIC_READ)
	if conin != syscall.InvalidHandle {
		err := windows.SetStdHandle(windows.STD_INPUT_HANDLE, windows.Handle(conin))
		if err != nil {
			return err
		}
		os.Stdin = os.NewFile(uintptr(conin), "CONIN$")
	}

	return nil
}

func createConsoleFile(name string, access uint32) syscall.Handle {
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return syscall.InvalidHandle
	}
	handle, err := windows.CreateFile(
		namePtr,
		access,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, // no security attributes
		windows.OPEN_EXISTING,
		0, // no flags
		0, // no template
	)
	if err != nil {
		return syscall.InvalidHandle
	}
	return syscall.Handle(handle)
}

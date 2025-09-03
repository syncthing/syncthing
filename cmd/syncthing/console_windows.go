// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build windows
// +build windows

package main

import (
	"os"
	"slices"
	"syscall"

	"golang.org/x/sys/windows"
)

var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procAllocConsole     = kernel32.NewProc("AllocConsole")
	procAttachConsole    = kernel32.NewProc("AttachConsole")
	procFreeConsole      = kernel32.NewProc("FreeConsole")
	procGetConsoleWindow = kernel32.NewProc("GetConsoleWindow")

	consoleAllocated = false
)

const (
	// Windows API constants
	ATTACH_PARENT_PROCESS = 0xFFFFFFFF
)

// InitConsole initializes console for Windows GUI applications
func InitConsole() error {
	// If this is an inner process (started by monitor) -> don't allocate console
	// as the monitor handles all I/O through pipes
	if os.Getenv("STMONITORED") == "yes" {
		return nil
	}

	// No command line arguments means binary was probably double-clicked -> don't allocate console
	if len(os.Args) <= 1 {
		return nil
	}

	// User explicitly disabled console  -> don't allocate console
	if slices.Contains(os.Args[1:], "--no-console") {
		return nil
	}

	// SSH sessions -> don't allocate console
	if os.Getenv("SSH_CLIENT") != "" || os.Getenv("SSH_TTY") != "" {
		return nil
	}

	// Console window already exists -> just redirect handles
	if hasConsole, _, _ := procGetConsoleWindow.Call(); hasConsole != 0 {
		return redirectStdHandles()
	}

	// Try to attach to parent consol
	if ret, _, err := procAttachConsole.Call(uintptr(ATTACH_PARENT_PROCESS)); ret != 0 {
		return redirectStdHandles()
	} else if err != syscall.Errno(0) {
		// Log the error but continue trying to allocate console
		// ERROR_ACCESS_DENIED (5) is expected when no parent console exists
		if errno, ok := err.(syscall.Errno); ok && errno != 5 {
			return nil // Don't fail completely, just skip console allocation
		}
	}

	// no parent console -> allocate a new one
	if ret, _, err := procAllocConsole.Call(); ret != 0 {
		consoleAllocated = true
		return redirectStdHandles()
	} else if err != syscall.Errno(0) {
		// ERROR_ACCESS_DENIED typically means console already exists
		if errno, ok := err.(syscall.Errno); ok && errno == 5 {
			return redirectStdHandles() // Try to redirect existing console
		}
	}

	return nil
}

func redirectStdHandles() error {
	// Create file handles for console output
	conout := createConsoleFile("CONOUT$", windows.GENERIC_WRITE|windows.GENERIC_READ)
	if conout != syscall.InvalidHandle {
		if err := windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(conout)); err == nil {
			os.Stdout = os.NewFile(uintptr(conout), "CONOUT$")
		}
	}

	// Create separate handle for stderr
	conerr := createConsoleFile("CONOUT$", windows.GENERIC_WRITE|windows.GENERIC_READ)
	if conerr != syscall.InvalidHandle {
		if err := windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(conerr)); err == nil {
			os.Stderr = os.NewFile(uintptr(conerr), "CONOUT$")
		}
	}

	// Create handle for console input
	conin := createConsoleFile("CONIN$", windows.GENERIC_READ)
	if conin != syscall.InvalidHandle {
		if err := windows.SetStdHandle(windows.STD_INPUT_HANDLE, windows.Handle(conin)); err == nil {
			os.Stdin = os.NewFile(uintptr(conin), "CONIN$")
		}
	}

	return nil
}

func createConsoleFile(name string, access uint32) syscall.Handle {
	namePtr, _ := syscall.UTF16PtrFromString(name)
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

// FreeConsole releases the console
func FreeConsole() {
	if consoleAllocated {
		procFreeConsole.Call()
		consoleAllocated = false
	}
}

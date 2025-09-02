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
)

const (
	// Windows API constants
	ATTACH_PARENT_PROCESS = 0xFFFFFFFF
)

var consoleAllocated = false

// InitConsole initializes console for Windows GUI applications
func InitConsole() error {
	// Only allocate console when we have actual command line arguments
	// os.Args[0] is always the program name, so we need more than 1 element
	if len(os.Args) <= 1 {
		return nil // No command line arguments, don't allocate console
	}

	// Check if --no-console flag is present
	if slices.Contains(os.Args[1:], "--no-console") {
			return nil // User explicitly disabled console
	}

	// Check if we already have a console window
	if hasConsole, _, _ := procGetConsoleWindow.Call(); hasConsole != 0 {
		// We have a console, but make sure handles are properly set
		return redirectStdHandles()
	}

	// Try to attach to parent console first (for command line usage)
	if ret, _, _ := procAttachConsole.Call(uintptr(ATTACH_PARENT_PROCESS)); ret != 0 {
		return redirectStdHandles()
	}

	// If no parent console, allocate a new one
	if ret, _, _ := procAllocConsole.Call(); ret != 0 {
		consoleAllocated = true
		return redirectStdHandles()
	}

	return nil
}

func redirectStdHandles() error {
	// Create file handles for console
	conout := createConsoleFile("CONOUT$", windows.GENERIC_WRITE|windows.GENERIC_READ)
	if conout != syscall.InvalidHandle {
		windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(conout))
		os.Stdout = os.NewFile(uintptr(conout), "CONOUT$")
	}

	conerr := createConsoleFile("CONOUT$", windows.GENERIC_WRITE|windows.GENERIC_READ)
	if conerr != syscall.InvalidHandle {
		windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(conerr))
		os.Stderr = os.NewFile(uintptr(conerr), "CONOUT$")
	}

	conin := createConsoleFile("CONIN$", windows.GENERIC_READ)
	if conin != syscall.InvalidHandle {
		windows.SetStdHandle(windows.STD_INPUT_HANDLE, windows.Handle(conin))
		os.Stdin = os.NewFile(uintptr(conin), "CONIN$")
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
	return syscall.Handle(handle)
}

// FreeConsole releases the console
func FreeConsole() {
	if consoleAllocated {
		procFreeConsole.Call()
		consoleAllocated = false
	}
}

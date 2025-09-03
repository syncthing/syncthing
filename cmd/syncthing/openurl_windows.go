// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build windows
// +build windows

package main

// Opening URLs on Windows: Why the complexity?
//
// While opening a URL seems simple, Windows provides multiple ways to do it,
// each with different security and compatibility trade-offs:
//
// 1. ShellExecuteW (preferred): Direct Windows API call that's secure and fast.
//    Uses the same mechanism that Windows Explorer uses internally.
//    Available on Windows NT 4.0+ (covers virtually all modern systems).
//
// 2. cmd.exe /C start (fallback): Traditional approach that spawns a command
//    shell. Less secure (potential command injection) but works on all Windows
//    versions including very old ones.
//
// This implementation tries the modern API first, then falls back to cmd.exe
// for maximum compatibility while maintaining security on modern systems.

import (
	"os/exec"
	"syscall"
	"unsafe"
)

var (
	shell32      = syscall.NewLazyDLL("shell32.dll")
	shellExecute = shell32.NewProc("ShellExecuteW")
)

func openURL(url string) error {
	// Try ShellExecuteW first - it's the most direct and secure approach
	if err := tryShellExecute(url); err == nil {
		return nil
	}

	// Fallback to cmd.exe for maximum compatibility (e.g., very old Windows versions)
	cmd := exec.Command("cmd.exe", "/C", "start", url)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

func tryShellExecute(url string) error {
	// Convert strings to UTF-16 for Windows API
	urlPtr, err := syscall.UTF16PtrFromString(url)
	if err != nil {
		return err
	}

	operationPtr, err := syscall.UTF16PtrFromString("open")
	if err != nil {
		return err
	}

	// Call ShellExecuteW
	ret, _, _ := shellExecute.Call(
		0,                                     // hwnd
		uintptr(unsafe.Pointer(operationPtr)), // lpOperation
		uintptr(unsafe.Pointer(urlPtr)),       // lpFile
		0,                                     // lpParameters
		0,                                     // lpDirectory
		1,                                     // nShowCmd (SW_SHOWNORMAL)
	)

	// ShellExecute returns a value > 32 on success
	if ret <= 32 {
		return syscall.Errno(ret)
	}

	return nil
}

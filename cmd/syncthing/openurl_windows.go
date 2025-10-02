// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build windows
// +build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	shell32      = syscall.NewLazyDLL("shell32.dll")
	shellExecute = shell32.NewProc("ShellExecuteW")
)

func openURL(url string) error {
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

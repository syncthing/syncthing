// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build windows
// +build windows

package osutil

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

const ATTACH_PARENT_PROCESS = -1

func AttachOrAllocateConsole() error {
	if hasConsoleWindow() {
		return errors.New("already attached")
	}

	var err error
	if err = attachConsole(ATTACH_PARENT_PROCESS); err != nil {
		if err = allocateConsole(); err != nil {
			bestEffortMessageBox("AttachOrAllocateConsole", fmt.Sprintf("allocate: %v", err))
			return err
		}
	}
	err = initConsoleHandles()
	if err != nil {
		bestEffortMessageBox("AttachOrAllocateConsole", fmt.Sprintf("init: %v", err))
	}
	return err
}

func hasConsoleWindow() bool {
	getConsoleWindow := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleWindow")
	if getConsoleWindow.Find() == nil {
		hwnd, _, _ := getConsoleWindow.Call()
		return hwnd != 0
	}
	return false // yolo
}

func allocateConsole() error {
	allocateConsole := syscall.NewLazyDLL("kernel32.dll").NewProc("AllocConsole")
	if allocateConsole.Find() == nil {
		success, _, _ := allocateConsole.Call()
		if success != 0 {
			return nil
		}
		lastError := windows.GetLastError()
		if lastError != nil {
			return lastError
		}
		return errors.New("failed to allocate console")
	}
	return errors.New("could not find AllocConsole")
}

func attachConsole(pid int) error {
	attachConsole := syscall.NewLazyDLL("kernel32.dll").NewProc("AttachConsole")
	if attachConsole.Find() == nil {
		success, _, _ := attachConsole.Call(uintptr(pid))
		if success != 0 {
			return nil
		}
		lastError := windows.GetLastError()
		if lastError != nil {
			return lastError
		}
		return errors.New("failed to attach console")
	}
	return errors.New("could not find AttachConsole")
}

func getFileType(fd windows.Handle) uintptr {
	getFileTypeCall := syscall.NewLazyDLL("kernel32.dll").NewProc("GetFileType")
	if getFileTypeCall.Find() == nil {
		t, _, _ := getFileTypeCall.Call(uintptr(fd))
		return t
	}
	return 9000
}

func initConsoleHandles() error {
	// Retrieve standard handles.
	hIn, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err != nil {
		return errors.New("Failed to retrieve standard input handler.")
	}
	hOut, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return errors.New("Failed to retrieve standard output handler.")
	}
	hErr, err := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
	if err != nil {
		return errors.New("Failed to retrieve standard error handler.")
	}

	// Wrap handles in files. /dev/ prefix just to make it conventional.
	stdInF := os.NewFile(uintptr(hIn), "/dev/stdin")
	if stdInF == nil {
		return errors.New("Failed to create a new file for standard input.")
	}
	stdOutF := os.NewFile(uintptr(hOut), "/dev/stdout")
	if stdOutF == nil {
		return errors.New("Failed to create a new file for standard output.")
	}
	stdErrF := os.NewFile(uintptr(hErr), "/dev/stderr")
	if stdErrF == nil {
		return errors.New("Failed to create a new file for standard error.")
	}

	// Set handles for standard input, output and error devices.
	err = windows.SetStdHandle(windows.STD_INPUT_HANDLE, windows.Handle(stdInF.Fd()))
	if err != nil {
		return errors.New("Failed to set standard input handler.")
	}
	err = windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(stdOutF.Fd()))
	if err != nil {
		return errors.New("Failed to set standard output handler.")
	}
	err = windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(stdErrF.Fd()))
	if err != nil {
		return errors.New("Failed to set standard error handler.")
	}

	// Update golang standard IO file descriptors.
	os.Stdin = stdInF
	os.Stdout = stdOutF
	os.Stderr = stdErrF

	return nil
}

func bestEffortMessageBox(title string, message string) {
	titlePtr, _ := windows.UTF16PtrFromString(title)
	messagePtr, _ := windows.UTF16PtrFromString(message)
	if titlePtr != nil && messagePtr != nil {
		_, _ = windows.MessageBox(0, messagePtr, titlePtr, 0)
	}
}

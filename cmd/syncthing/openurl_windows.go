// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build windows
// +build windows

package main

import "golang.org/x/sys/windows"

func openURL(url string) error {
	urlPtr, err := windows.UTF16PtrFromString(url)
	if err != nil {
		return err
	}

	verbPtr, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}

	err = windows.ShellExecute(
		0,       // hwnd
		verbPtr, // operation
		urlPtr,  // file
		nil,     // parameters
		nil,     // directory
		windows.SW_SHOWNORMAL,
	)

	return err
}

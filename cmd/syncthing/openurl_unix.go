// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build !windows

package main

import (
	"os/exec"
	"runtime"
	"syscall"
)

func openURL(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Run()

	default:
		cmd := exec.Command("xdg-open", url)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true,
		}
		return cmd.Run()
	}
}

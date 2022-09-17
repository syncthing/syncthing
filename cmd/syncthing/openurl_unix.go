// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !windows
// +build !windows

package toplevel

import (
	"os/exec"
	"syscall"

	"github.com/syncthing/syncthing/lib/build"
)

func openURL(url string) error {
	if build.IsDarwin {
		return exec.Command("open", url).Run()
	}
	cmd := exec.Command("xdg-open", url)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	return cmd.Run()
}

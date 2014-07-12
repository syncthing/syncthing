// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build windows

package main

import "os/exec"

func openURL(url string) error {
	return exec.Command("cmd.exe", "/C", "start "+url).Run()
}

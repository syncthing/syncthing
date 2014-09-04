// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build solaris

package main

import (
	"os/exec"
	"strconv"
)

func memorySize() (uint64, error) {
	cmd := exec.Command("prtconf", "-m")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, err
	}

	mb, err := strconv.ParseUint(string(out), 10, 64)
	if err != nil {
		return 0, err
	}
	return mb * 1024 * 1024, nil
}

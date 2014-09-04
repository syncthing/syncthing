// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
)

func memorySize() (uint64, error) {
	cmd := exec.Command("sysctl", "hw.memsize")
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	fs := strings.Fields(string(out))
	if len(fs) != 2 {
		return 0, errors.New("sysctl parse error")
	}
	bytes, err := strconv.ParseUint(fs[1], 10, 64)
	if err != nil {
		return 0, err
	}
	return bytes, nil
}

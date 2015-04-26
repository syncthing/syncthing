// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
)

func memorySize() (int64, error) {
	cmd := exec.Command("sysctl", "hw.memsize")
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	fs := strings.Fields(string(out))
	if len(fs) != 2 {
		return 0, errors.New("sysctl parse error")
	}
	bytes, err := strconv.ParseInt(fs[1], 10, 64)
	if err != nil {
		return 0, err
	}
	return bytes, nil
}

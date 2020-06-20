// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package ur

import (
	"os/exec"
	"strconv"
	"strings"
)

func memorySize() int64 {
	cmd := exec.Command("/sbin/sysctl", "hw.physmem64")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	fs := strings.Fields(string(out))
	if len(fs) != 3 {
		return 0
	}
	bytes, err := strconv.ParseInt(fs[2], 10, 64)
	if err != nil {
		return 0
	}
	return bytes
}

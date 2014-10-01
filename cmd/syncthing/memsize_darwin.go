// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

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

// Copyright (C) 2014 The Syncthing Authors.
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

// +build solaris

package main

import (
	"os/exec"
	"strconv"
)

func memorySize() (int64, error) {
	cmd := exec.Command("prtconf", "-m")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, err
	}

	mb, err := strconv.ParseInt(string(out), 10, 64)
	if err != nil {
		return 0, err
	}
	return mb * 1024 * 1024, nil
}

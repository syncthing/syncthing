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

package main

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
)

func memorySize() (int64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}

	s := bufio.NewScanner(f)
	if !s.Scan() {
		return 0, errors.New("/proc/meminfo parse error 1")
	}

	l := s.Text()
	fs := strings.Fields(l)
	if len(fs) != 3 || fs[2] != "kB" {
		return 0, errors.New("/proc/meminfo parse error 2")
	}

	kb, err := strconv.ParseInt(fs[1], 10, 64)
	if err != nil {
		return 0, err
	}
	return kb * 1024, nil
}

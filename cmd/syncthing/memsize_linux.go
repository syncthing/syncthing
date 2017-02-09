// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

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

// Copyright (C) 2022 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build linux || darwin
// +build linux darwin

package fs

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/unix"
)

func listXattr(path string) ([]string, error) {
	buf := make([]byte, 1024)
	size, err := unix.Llistxattr(path, buf)
	if errors.Is(err, unix.ERANGE) {
		// Buffer is too small. Try again with a zero sized buffer to get
		// the size, then allocate a buffer of the correct size.
		size, err = unix.Llistxattr(path, nil)
		if err != nil {
			return nil, fmt.Errorf("Listxattr %q: %w", path, err)
		}
		if size > len(buf) {
			buf = make([]byte, size)
		}
		size, err = unix.Llistxattr(path, buf)
	}
	if err != nil {
		return nil, fmt.Errorf("Listxattr %q: %w", path, err)
	}

	buf = buf[:size]
	attrs := strings.Split(string(buf), "\x00")

	return attrs, nil
}

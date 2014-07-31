// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package osutil implements utilities for native OS support.
package osutil

import (
	"os"
	"runtime"
)

func Rename(from, to string) error {
	if runtime.GOOS == "windows" {
		os.Chmod(to, 0666) // Make sure the file is user writeable
		err := os.Remove(to)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	defer os.Remove(from) // Don't leave a dangling temp file in case of rename error
	return os.Rename(from, to)
}

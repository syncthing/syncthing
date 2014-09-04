// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build freebsd

package main

import "errors"

func memorySize() (uint64, error) {
	return 0, errors.New("not implemented")
}

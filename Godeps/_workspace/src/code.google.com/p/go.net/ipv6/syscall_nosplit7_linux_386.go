// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.1,!go1.2

package ipv6

import "syscall"

func socketcallnosplit7(call int, a0, a1, a2, a3, a4, a5 uintptr) (int, syscall.Errno)

func init() {
	socketcall = socketcallnosplit7
}

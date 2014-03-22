// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin freebsd linux netbsd openbsd

package ipv6

import "syscall"

const sysSizeofMTUInfo = 0x20

type sysMTUInfo struct {
	Addr syscall.RawSockaddrInet6
	MTU  uint32
}

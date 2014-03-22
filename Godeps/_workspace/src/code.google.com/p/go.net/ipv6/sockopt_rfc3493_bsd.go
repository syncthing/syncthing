// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin freebsd netbsd openbsd

package ipv6

import (
	"os"
	"unsafe"
)

func setIPv6Checksum(fd int, on bool, offset int) error {
	if !on {
		offset = -1
	}
	v := int32(offset)
	return os.NewSyscallError("setsockopt", setsockopt(fd, ianaProtocolIPv6, sysSockoptChecksum, uintptr(unsafe.Pointer(&v)), 4))
}

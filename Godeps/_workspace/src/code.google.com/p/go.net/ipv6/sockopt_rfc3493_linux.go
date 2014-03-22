// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

import (
	"os"
	"syscall"
)

func setIPv6Checksum(fd int, on bool, offset int) error {
	if !on {
		offset = -1
	}
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, ianaProtocolReserved, syscall.IPV6_CHECKSUM, offset))
}

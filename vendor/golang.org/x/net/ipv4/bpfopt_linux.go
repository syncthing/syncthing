// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv4

import (
	"os"
	"unsafe"

	"golang.org/x/net/bpf"
	"golang.org/x/net/internal/netreflect"
)

// SetBPF attaches a BPF program to the connection.
//
// Only supported on Linux.
func (c *dgramOpt) SetBPF(filter []bpf.RawInstruction) error {
	s, err := netreflect.PacketSocketOf(c.PacketConn)
	if err != nil {
		return err
	}
	prog := sockFProg{
		Len:    uint16(len(filter)),
		Filter: (*sockFilter)(unsafe.Pointer(&filter[0])),
	}
	return os.NewSyscallError("setsockopt", setsockopt(s, sysSOL_SOCKET, sysSO_ATTACH_FILTER, unsafe.Pointer(&prog), uint32(unsafe.Sizeof(prog))))
}

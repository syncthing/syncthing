// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

import "syscall"

func ipv6ReceiveTrafficClass(fd syscall.Handle) (bool, error) {
	// TODO(mikio): Implement this
	return false, syscall.EWINDOWS
}

func setIPv6ReceiveTrafficClass(fd syscall.Handle, v bool) error {
	// TODO(mikio): Implement this
	return syscall.EWINDOWS
}

func ipv6ReceiveHopLimit(fd syscall.Handle) (bool, error) {
	// TODO(mikio): Implement this
	return false, syscall.EWINDOWS
}

func setIPv6ReceiveHopLimit(fd syscall.Handle, v bool) error {
	// TODO(mikio): Implement this
	return syscall.EWINDOWS
}

func ipv6ReceivePacketInfo(fd syscall.Handle) (bool, error) {
	// TODO(mikio): Implement this
	return false, syscall.EWINDOWS
}

func setIPv6ReceivePacketInfo(fd syscall.Handle, v bool) error {
	// TODO(mikio): Implement this
	return syscall.EWINDOWS
}

func ipv6PathMTU(fd syscall.Handle) (int, error) {
	// TODO(mikio): Implement this
	return 0, syscall.EWINDOWS
}

func ipv6ReceivePathMTU(fd syscall.Handle) (bool, error) {
	// TODO(mikio): Implement this
	return false, syscall.EWINDOWS
}

func setIPv6ReceivePathMTU(fd syscall.Handle, v bool) error {
	// TODO(mikio): Implement this
	return syscall.EWINDOWS
}

func ipv6ICMPFilter(fd syscall.Handle) (*ICMPFilter, error) {
	// TODO(mikio): Implement this
	return nil, syscall.EWINDOWS
}

func setIPv6ICMPFilter(fd syscall.Handle, f *ICMPFilter) error {
	// TODO(mikio): Implement this
	return syscall.EWINDOWS
}

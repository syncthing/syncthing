// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

import (
	"net"
	"syscall"
)

// MulticastHopLimit returns the hop limit field value for outgoing
// multicast packets.
func (c *dgramOpt) MulticastHopLimit() (int, error) {
	// TODO(mikio): Implement this
	return 0, syscall.EPLAN9
}

// SetMulticastHopLimit sets the hop limit field value for future
// outgoing multicast packets.
func (c *dgramOpt) SetMulticastHopLimit(hoplim int) error {
	// TODO(mikio): Implement this
	return syscall.EPLAN9
}

// MulticastInterface returns the default interface for multicast
// packet transmissions.
func (c *dgramOpt) MulticastInterface() (*net.Interface, error) {
	// TODO(mikio): Implement this
	return nil, syscall.EPLAN9
}

// SetMulticastInterface sets the default interface for future
// multicast packet transmissions.
func (c *dgramOpt) SetMulticastInterface(ifi *net.Interface) error {
	// TODO(mikio): Implement this
	return syscall.EPLAN9
}

// MulticastLoopback reports whether transmitted multicast packets
// should be copied and send back to the originator.
func (c *dgramOpt) MulticastLoopback() (bool, error) {
	// TODO(mikio): Implement this
	return false, syscall.EPLAN9
}

// SetMulticastLoopback sets whether transmitted multicast packets
// should be copied and send back to the originator.
func (c *dgramOpt) SetMulticastLoopback(on bool) error {
	// TODO(mikio): Implement this
	return syscall.EPLAN9
}

// JoinGroup joins the group address group on the interface ifi.
// It uses the system assigned multicast interface when ifi is nil,
// although this is not recommended because the assignment depends on
// platforms and sometimes it might require routing configuration.
func (c *dgramOpt) JoinGroup(ifi *net.Interface, group net.Addr) error {
	// TODO(mikio): Implement this
	return syscall.EPLAN9
}

// LeaveGroup leaves the group address group on the interface ifi.
func (c *dgramOpt) LeaveGroup(ifi *net.Interface, group net.Addr) error {
	// TODO(mikio): Implement this
	return syscall.EPLAN9
}

// Checksum reports whether the kernel will compute, store or verify a
// checksum for both incoming and outgoing packets.  If on is true, it
// returns an offset in bytes into the data of where the checksum
// field is located.
func (c *dgramOpt) Checksum() (on bool, offset int, err error) {
	// TODO(mikio): Implement this
	return false, 0, syscall.EPLAN9
}

// SetChecksum enables the kernel checksum processing.  If on is ture,
// the offset should be an offset in bytes into the data of where the
// checksum field is located.
func (c *dgramOpt) SetChecksum(on bool, offset int) error {
	// TODO(mikio): Implement this
	return syscall.EPLAN9
}

// ICMPFilter returns an ICMP filter.
func (c *dgramOpt) ICMPFilter() (*ICMPFilter, error) {
	// TODO(mikio): Implement this
	return nil, syscall.EPLAN9
}

// SetICMPFilter deploys the ICMP filter.
func (c *dgramOpt) SetICMPFilter(f *ICMPFilter) error {
	// TODO(mikio): Implement this
	return syscall.EPLAN9
}

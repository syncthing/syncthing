// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin freebsd linux netbsd openbsd windows

package ipv6

import (
	"net"
	"syscall"
)

// MulticastHopLimit returns the hop limit field value for outgoing
// multicast packets.
func (c *dgramOpt) MulticastHopLimit() (int, error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}
	fd, err := c.sysfd()
	if err != nil {
		return 0, err
	}
	return ipv6MulticastHopLimit(fd)
}

// SetMulticastHopLimit sets the hop limit field value for future
// outgoing multicast packets.
func (c *dgramOpt) SetMulticastHopLimit(hoplim int) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	fd, err := c.sysfd()
	if err != nil {
		return err
	}
	return setIPv6MulticastHopLimit(fd, hoplim)
}

// MulticastInterface returns the default interface for multicast
// packet transmissions.
func (c *dgramOpt) MulticastInterface() (*net.Interface, error) {
	if !c.ok() {
		return nil, syscall.EINVAL
	}
	fd, err := c.sysfd()
	if err != nil {
		return nil, err
	}
	return ipv6MulticastInterface(fd)
}

// SetMulticastInterface sets the default interface for future
// multicast packet transmissions.
func (c *dgramOpt) SetMulticastInterface(ifi *net.Interface) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	fd, err := c.sysfd()
	if err != nil {
		return err
	}
	return setIPv6MulticastInterface(fd, ifi)
}

// MulticastLoopback reports whether transmitted multicast packets
// should be copied and send back to the originator.
func (c *dgramOpt) MulticastLoopback() (bool, error) {
	if !c.ok() {
		return false, syscall.EINVAL
	}
	fd, err := c.sysfd()
	if err != nil {
		return false, err
	}
	return ipv6MulticastLoopback(fd)
}

// SetMulticastLoopback sets whether transmitted multicast packets
// should be copied and send back to the originator.
func (c *dgramOpt) SetMulticastLoopback(on bool) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	fd, err := c.sysfd()
	if err != nil {
		return err
	}
	return setIPv6MulticastLoopback(fd, on)
}

// JoinGroup joins the group address group on the interface ifi.
// It uses the system assigned multicast interface when ifi is nil,
// although this is not recommended because the assignment depends on
// platforms and sometimes it might require routing configuration.
func (c *dgramOpt) JoinGroup(ifi *net.Interface, group net.Addr) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	fd, err := c.sysfd()
	if err != nil {
		return err
	}
	grp := netAddrToIP16(group)
	if grp == nil {
		return errMissingAddress
	}
	return joinIPv6Group(fd, ifi, grp)
}

// LeaveGroup leaves the group address group on the interface ifi.
func (c *dgramOpt) LeaveGroup(ifi *net.Interface, group net.Addr) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	fd, err := c.sysfd()
	if err != nil {
		return err
	}
	grp := netAddrToIP16(group)
	if grp == nil {
		return errMissingAddress
	}
	return leaveIPv6Group(fd, ifi, grp)
}

// Checksum reports whether the kernel will compute, store or verify a
// checksum for both incoming and outgoing packets.  If on is true, it
// returns an offset in bytes into the data of where the checksum
// field is located.
func (c *dgramOpt) Checksum() (on bool, offset int, err error) {
	if !c.ok() {
		return false, 0, syscall.EINVAL
	}
	fd, err := c.sysfd()
	if err != nil {
		return false, 0, err
	}
	return ipv6Checksum(fd)
}

// SetChecksum enables the kernel checksum processing.  If on is ture,
// the offset should be an offset in bytes into the data of where the
// checksum field is located.
func (c *dgramOpt) SetChecksum(on bool, offset int) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	fd, err := c.sysfd()
	if err != nil {
		return err
	}
	return setIPv6Checksum(fd, on, offset)
}

// ICMPFilter returns an ICMP filter.
func (c *dgramOpt) ICMPFilter() (*ICMPFilter, error) {
	if !c.ok() {
		return nil, syscall.EINVAL
	}
	fd, err := c.sysfd()
	if err != nil {
		return nil, err
	}
	return ipv6ICMPFilter(fd)
}

// SetICMPFilter deploys the ICMP filter.
func (c *dgramOpt) SetICMPFilter(f *ICMPFilter) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	fd, err := c.sysfd()
	if err != nil {
		return err
	}
	return setIPv6ICMPFilter(fd, f)
}

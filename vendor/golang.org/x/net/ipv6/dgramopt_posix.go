// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin dragonfly freebsd linux netbsd openbsd windows

package ipv6

import (
	"net"
	"syscall"

	"golang.org/x/net/internal/netreflect"
)

// MulticastHopLimit returns the hop limit field value for outgoing
// multicast packets.
func (c *dgramOpt) MulticastHopLimit() (int, error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}
	s, err := netreflect.PacketSocketOf(c.PacketConn)
	if err != nil {
		return 0, err
	}
	return getInt(s, &sockOpts[ssoMulticastHopLimit])
}

// SetMulticastHopLimit sets the hop limit field value for future
// outgoing multicast packets.
func (c *dgramOpt) SetMulticastHopLimit(hoplim int) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	s, err := netreflect.PacketSocketOf(c.PacketConn)
	if err != nil {
		return err
	}
	return setInt(s, &sockOpts[ssoMulticastHopLimit], hoplim)
}

// MulticastInterface returns the default interface for multicast
// packet transmissions.
func (c *dgramOpt) MulticastInterface() (*net.Interface, error) {
	if !c.ok() {
		return nil, syscall.EINVAL
	}
	s, err := netreflect.PacketSocketOf(c.PacketConn)
	if err != nil {
		return nil, err
	}
	return getInterface(s, &sockOpts[ssoMulticastInterface])
}

// SetMulticastInterface sets the default interface for future
// multicast packet transmissions.
func (c *dgramOpt) SetMulticastInterface(ifi *net.Interface) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	s, err := netreflect.PacketSocketOf(c.PacketConn)
	if err != nil {
		return err
	}
	return setInterface(s, &sockOpts[ssoMulticastInterface], ifi)
}

// MulticastLoopback reports whether transmitted multicast packets
// should be copied and send back to the originator.
func (c *dgramOpt) MulticastLoopback() (bool, error) {
	if !c.ok() {
		return false, syscall.EINVAL
	}
	s, err := netreflect.PacketSocketOf(c.PacketConn)
	if err != nil {
		return false, err
	}
	on, err := getInt(s, &sockOpts[ssoMulticastLoopback])
	if err != nil {
		return false, err
	}
	return on == 1, nil
}

// SetMulticastLoopback sets whether transmitted multicast packets
// should be copied and send back to the originator.
func (c *dgramOpt) SetMulticastLoopback(on bool) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	s, err := netreflect.PacketSocketOf(c.PacketConn)
	if err != nil {
		return err
	}
	return setInt(s, &sockOpts[ssoMulticastLoopback], boolint(on))
}

// JoinGroup joins the group address group on the interface ifi.
// By default all sources that can cast data to group are accepted.
// It's possible to mute and unmute data transmission from a specific
// source by using ExcludeSourceSpecificGroup and
// IncludeSourceSpecificGroup.
// JoinGroup uses the system assigned multicast interface when ifi is
// nil, although this is not recommended because the assignment
// depends on platforms and sometimes it might require routing
// configuration.
func (c *dgramOpt) JoinGroup(ifi *net.Interface, group net.Addr) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	s, err := netreflect.PacketSocketOf(c.PacketConn)
	if err != nil {
		return err
	}
	grp := netAddrToIP16(group)
	if grp == nil {
		return errMissingAddress
	}
	return setGroup(s, &sockOpts[ssoJoinGroup], ifi, grp)
}

// LeaveGroup leaves the group address group on the interface ifi
// regardless of whether the group is any-source group or
// source-specific group.
func (c *dgramOpt) LeaveGroup(ifi *net.Interface, group net.Addr) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	s, err := netreflect.PacketSocketOf(c.PacketConn)
	if err != nil {
		return err
	}
	grp := netAddrToIP16(group)
	if grp == nil {
		return errMissingAddress
	}
	return setGroup(s, &sockOpts[ssoLeaveGroup], ifi, grp)
}

// JoinSourceSpecificGroup joins the source-specific group comprising
// group and source on the interface ifi.
// JoinSourceSpecificGroup uses the system assigned multicast
// interface when ifi is nil, although this is not recommended because
// the assignment depends on platforms and sometimes it might require
// routing configuration.
func (c *dgramOpt) JoinSourceSpecificGroup(ifi *net.Interface, group, source net.Addr) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	s, err := netreflect.PacketSocketOf(c.PacketConn)
	if err != nil {
		return err
	}
	grp := netAddrToIP16(group)
	if grp == nil {
		return errMissingAddress
	}
	src := netAddrToIP16(source)
	if src == nil {
		return errMissingAddress
	}
	return setSourceGroup(s, &sockOpts[ssoJoinSourceGroup], ifi, grp, src)
}

// LeaveSourceSpecificGroup leaves the source-specific group on the
// interface ifi.
func (c *dgramOpt) LeaveSourceSpecificGroup(ifi *net.Interface, group, source net.Addr) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	s, err := netreflect.PacketSocketOf(c.PacketConn)
	if err != nil {
		return err
	}
	grp := netAddrToIP16(group)
	if grp == nil {
		return errMissingAddress
	}
	src := netAddrToIP16(source)
	if src == nil {
		return errMissingAddress
	}
	return setSourceGroup(s, &sockOpts[ssoLeaveSourceGroup], ifi, grp, src)
}

// ExcludeSourceSpecificGroup excludes the source-specific group from
// the already joined any-source groups by JoinGroup on the interface
// ifi.
func (c *dgramOpt) ExcludeSourceSpecificGroup(ifi *net.Interface, group, source net.Addr) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	s, err := netreflect.PacketSocketOf(c.PacketConn)
	if err != nil {
		return err
	}
	grp := netAddrToIP16(group)
	if grp == nil {
		return errMissingAddress
	}
	src := netAddrToIP16(source)
	if src == nil {
		return errMissingAddress
	}
	return setSourceGroup(s, &sockOpts[ssoBlockSourceGroup], ifi, grp, src)
}

// IncludeSourceSpecificGroup includes the excluded source-specific
// group by ExcludeSourceSpecificGroup again on the interface ifi.
func (c *dgramOpt) IncludeSourceSpecificGroup(ifi *net.Interface, group, source net.Addr) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	s, err := netreflect.PacketSocketOf(c.PacketConn)
	if err != nil {
		return err
	}
	grp := netAddrToIP16(group)
	if grp == nil {
		return errMissingAddress
	}
	src := netAddrToIP16(source)
	if src == nil {
		return errMissingAddress
	}
	return setSourceGroup(s, &sockOpts[ssoUnblockSourceGroup], ifi, grp, src)
}

// Checksum reports whether the kernel will compute, store or verify a
// checksum for both incoming and outgoing packets.  If on is true, it
// returns an offset in bytes into the data of where the checksum
// field is located.
func (c *dgramOpt) Checksum() (on bool, offset int, err error) {
	if !c.ok() {
		return false, 0, syscall.EINVAL
	}
	s, err := netreflect.PacketSocketOf(c.PacketConn)
	if err != nil {
		return false, 0, err
	}
	offset, err = getInt(s, &sockOpts[ssoChecksum])
	if err != nil {
		return false, 0, err
	}
	if offset < 0 {
		return false, 0, nil
	}
	return true, offset, nil
}

// SetChecksum enables the kernel checksum processing.  If on is ture,
// the offset should be an offset in bytes into the data of where the
// checksum field is located.
func (c *dgramOpt) SetChecksum(on bool, offset int) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	s, err := netreflect.PacketSocketOf(c.PacketConn)
	if err != nil {
		return err
	}
	if !on {
		offset = -1
	}
	return setInt(s, &sockOpts[ssoChecksum], offset)
}

// ICMPFilter returns an ICMP filter.
func (c *dgramOpt) ICMPFilter() (*ICMPFilter, error) {
	if !c.ok() {
		return nil, syscall.EINVAL
	}
	s, err := netreflect.PacketSocketOf(c.PacketConn)
	if err != nil {
		return nil, err
	}
	return getICMPFilter(s, &sockOpts[ssoICMPFilter])
}

// SetICMPFilter deploys the ICMP filter.
func (c *dgramOpt) SetICMPFilter(f *ICMPFilter) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	s, err := netreflect.PacketSocketOf(c.PacketConn)
	if err != nil {
		return err
	}
	return setICMPFilter(s, &sockOpts[ssoICMPFilter], f)
}

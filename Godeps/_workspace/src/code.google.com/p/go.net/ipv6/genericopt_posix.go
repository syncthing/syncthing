// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin freebsd linux netbsd openbsd windows

package ipv6

import "syscall"

// TrafficClass returns the traffic class field value for outgoing
// packets.
func (c *genericOpt) TrafficClass() (int, error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}
	fd, err := c.sysfd()
	if err != nil {
		return 0, err
	}
	return ipv6TrafficClass(fd)
}

// SetTrafficClass sets the traffic class field value for future
// outgoing packets.
func (c *genericOpt) SetTrafficClass(tclass int) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	fd, err := c.sysfd()
	if err != nil {
		return err
	}
	return setIPv6TrafficClass(fd, tclass)
}

// HopLimit returns the hop limit field value for outgoing packets.
func (c *genericOpt) HopLimit() (int, error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}
	fd, err := c.sysfd()
	if err != nil {
		return 0, err
	}
	return ipv6HopLimit(fd)
}

// SetHopLimit sets the hop limit field value for future outgoing
// packets.
func (c *genericOpt) SetHopLimit(hoplim int) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	fd, err := c.sysfd()
	if err != nil {
		return err
	}
	return setIPv6HopLimit(fd, hoplim)
}

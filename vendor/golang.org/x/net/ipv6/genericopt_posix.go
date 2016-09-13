// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin dragonfly freebsd linux netbsd openbsd windows

package ipv6

import (
	"syscall"

	"golang.org/x/net/internal/netreflect"
)

// TrafficClass returns the traffic class field value for outgoing
// packets.
func (c *genericOpt) TrafficClass() (int, error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}
	s, err := netreflect.SocketOf(c.Conn)
	if err != nil {
		return 0, err
	}
	return getInt(s, &sockOpts[ssoTrafficClass])
}

// SetTrafficClass sets the traffic class field value for future
// outgoing packets.
func (c *genericOpt) SetTrafficClass(tclass int) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	s, err := netreflect.SocketOf(c.Conn)
	if err != nil {
		return err
	}
	return setInt(s, &sockOpts[ssoTrafficClass], tclass)
}

// HopLimit returns the hop limit field value for outgoing packets.
func (c *genericOpt) HopLimit() (int, error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}
	s, err := netreflect.SocketOf(c.Conn)
	if err != nil {
		return 0, err
	}
	return getInt(s, &sockOpts[ssoHopLimit])
}

// SetHopLimit sets the hop limit field value for future outgoing
// packets.
func (c *genericOpt) SetHopLimit(hoplim int) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	s, err := netreflect.SocketOf(c.Conn)
	if err != nil {
		return err
	}
	return setInt(s, &sockOpts[ssoHopLimit], hoplim)
}

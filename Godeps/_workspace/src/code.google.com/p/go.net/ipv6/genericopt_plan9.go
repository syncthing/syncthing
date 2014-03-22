// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipv6

import "syscall"

// TrafficClass returns the traffic class field value for outgoing
// packets.
func (c *genericOpt) TrafficClass() (int, error) {
	// TODO(mikio): Implement this
	return 0, syscall.EPLAN9
}

// SetTrafficClass sets the traffic class field value for future
// outgoing packets.
func (c *genericOpt) SetTrafficClass(tclass int) error {
	// TODO(mikio): Implement this
	return syscall.EPLAN9
}

// HopLimit returns the hop limit field value for outgoing packets.
func (c *genericOpt) HopLimit() (int, error) {
	// TODO(mikio): Implement this
	return 0, syscall.EPLAN9
}

// SetHopLimit sets the hop limit field value for future outgoing
// packets.
func (c *genericOpt) SetHopLimit(hoplim int) error {
	// TODO(mikio): Implement this
	return syscall.EPLAN9
}

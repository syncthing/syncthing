// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package nat

import (
	"net"
	"time"
)

type Protocol string

const (
	TCP Protocol = "TCP"
	UDP          = "UDP"
)

type Device interface {
	ID() string
	GetLocalIPAddress() net.IP
	AddPortMapping(protocol Protocol, internalPort, externalPort int, description string, duration time.Duration) (int, error)
	GetExternalIPAddress() (net.IP, error)
}

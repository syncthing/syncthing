// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package nat

import (
	"context"
	"net"
	"time"
)

type Protocol string

const (
	TCP Protocol = "TCP"
	UDP Protocol = "UDP"
)

type Device interface {
	ID() string
	GetLocalIPAddress() net.IP
	AddPortMapping(ctx context.Context, protocol Protocol, internalPort, externalPort int, description string, duration time.Duration) (int, error)
	TryAddPinhole(ctx context.Context, protocol Protocol, port int, description string, duration time.Duration) (int, error)
	GetExternalIPAddress(ctx context.Context) (net.IP, error)
}

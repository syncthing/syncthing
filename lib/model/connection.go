// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package model

import (
	"crypto/tls"
	"net"

	"github.com/syncthing/protocol"
)

type IntermediateConnection struct {
	*tls.Conn
	Type ConnectionType
}

type Connection struct {
	net.Conn
	protocol.Connection
	Type ConnectionType
}

const (
	ConnectionTypeDirectAccept ConnectionType = iota
	ConnectionTypeDirectDial
	ConnectionTypeRelayAccept
	ConnectionTypeRelayDial
)

type ConnectionType int

func (t ConnectionType) String() string {
	switch t {
	case ConnectionTypeDirectAccept:
		return "direct-accept"
	case ConnectionTypeDirectDial:
		return "direct-dial"
	case ConnectionTypeRelayAccept:
		return "relay-accept"
	case ConnectionTypeRelayDial:
		return "relay-dial"
	}
	return "unknown"
}

func (t ConnectionType) IsDirect() bool {
	return t == ConnectionTypeDirectAccept || t == ConnectionTypeDirectDial
}

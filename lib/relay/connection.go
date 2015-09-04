// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package relay

import "crypto/tls"

type Connection struct {
	*tls.Conn
	Type ConnectionType
}

func (c Connection) IsDirect() bool {
	return c.Type == ConnectionTypeDirectAccept || c.Type == ConnectionTypeDirectDial
}

type ConnectionType int

const (
	ConnectionTypeDirectAccept ConnectionType = iota
	ConnectionTypeDirectDial
	ConnectionTypeRelayAccept
	ConnectionTypeRelayDial
)

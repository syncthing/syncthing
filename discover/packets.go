// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package discover

const (
	AnnouncementMagic = 0x9D79BC39
	QueryMagic        = 0x2CA856F5
)

type Query struct {
	Magic  uint32
	NodeID []byte // max:32
}

type Announce struct {
	Magic uint32
	This  Node
	Extra []Node // max:16
}

type Node struct {
	ID        []byte    // max:32
	Addresses []Address // max:16
}

type Address struct {
	IP   []byte // max:16
	Port uint16
}

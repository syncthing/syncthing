// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//go:generate -command genxdr go run ../../Godeps/_workspace/src/github.com/calmh/xdr/cmd/genxdr/main.go
//go:generate genxdr -o packets_xdr.go packets.go

package discover

const (
	AnnouncementMagic = 0x9D79BC40
	QueryMagic        = 0x2CA856F6
)

type Query struct {
	Magic    uint32
	DeviceID []byte // max:32
}

type Announce struct {
	Magic uint32
	This  Device
	Extra []Device // max:16
}

type Relay struct {
	Address string // max:256
	Latency int32
}

type Device struct {
	ID        []byte   // max:32
	Addresses []string // max:16
	Relays    []Relay  // max:16
}

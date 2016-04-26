// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

//go:generate -command genxdr go run ../../vendor/github.com/calmh/xdr/cmd/genxdr/main.go
//go:generate genxdr -o localpackets_xdr.go localpackets.go

package discover

const (
	AnnouncementMagic = 0x7D79BC40
)

type Announce struct {
	Magic uint32
	This  Device
	Extra []Device // max:16
}

type Device struct {
	ID        []byte    // max:32
	Addresses []Address // max:16
}

type Address struct {
	URL string // max:2083
}

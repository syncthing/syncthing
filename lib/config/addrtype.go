// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import "strings"

type AddressType int

const (
	AddressTypeTCP AddressType = iota // default is TCP
	AddressTypeUNIX
)

func (t AddressType) String() string {
	switch t {
	case AddressTypeTCP:
		return "tcp"
	case AddressTypeUNIX:
		return "unix"
	default:
		return "unknown"
	}
}

func (t AddressType) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

func (t *AddressType) UnmarshalText(bs []byte) error {
	switch strings.ToLower(string(bs)) {
	case "tcp":
		*t = AddressTypeTCP
	case "unix":
		*t = AddressTypeUNIX
	default:
		*t = AddressTypeTCP
	}
	return nil
}

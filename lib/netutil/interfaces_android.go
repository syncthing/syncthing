// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package netutil

import (
	"net"

	"github.com/wlynxg/anet"
)

func Interfaces() ([]net.Interface, error) {
	return anet.Interfaces()
}

func InterfaceAddrsByInterface(intf *net.Interface) ([]net.Addr, error) {
	return anet.InterfaceAddrsByInterface(intf)
}

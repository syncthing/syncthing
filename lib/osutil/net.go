// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package osutil

import (
	"net"
)

func interfaceAddresses(network string, intf *net.Interface) []string {
	var out []string
	addrs, err := intf.Addrs()
	if err != nil {
		return out
	}

	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if ok && (network == "tcp" || (network == "tcp4" && len(ipnet.IP) == net.IPv4len) || (network == "tcp6" && len(ipnet.IP) == net.IPv6len)) {
			out = append(out, ipnet.IP.String())
		}
	}

	return out
}

// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil

import (
	"net"
	"strings"

	"github.com/syncthing/syncthing/lib/netutil"
)

// GetInterfaceAddrs returns the IP networks of all interfaces that are up.
// Point-to-point interfaces are excluded unless includePtP is true.
func GetInterfaceAddrs(includePtP bool) ([]*net.IPNet, error) {
	intfs, err := netutil.Interfaces()
	if err != nil {
		return nil, err
	}
	var addrs []net.Addr

	for i := range intfs {
		intf := intfs[i]

		if intf.Flags&net.FlagRunning == 0 {
			continue
		}
		if !includePtP && intf.Flags&net.FlagPointToPoint != 0 {
			// Point-to-point interfaces are typically VPNs and similar
			// which, for our purposes, do not qualify as LANs.
			continue
		}
		intfAddrs, err := netutil.InterfaceAddrsByInterface(&intf)
		if err != nil {
			return nil, err
		}
		addrs = append(addrs, intfAddrs...)
	}

	nets := make([]*net.IPNet, 0, len(addrs))

	for _, addr := range addrs {
		net, ok := addr.(*net.IPNet)
		if ok {
			nets = append(nets, net)
		}
	}
	return nets, nil
}

func IPFromString(addr string) net.IP {
	// strip the port
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	// strip IPv6 zone identifier
	host, _, _ = strings.Cut(host, "%")
	return net.ParseIP(host)
}

func IPFromAddr(addr net.Addr) (net.IP, error) {
	switch a := addr.(type) {
	case *net.TCPAddr:
		return a.IP, nil
	case *net.UDPAddr:
		return a.IP, nil
	default:
		host, _, err := net.SplitHostPort(addr.String())
		return net.ParseIP(host), err
	}
}

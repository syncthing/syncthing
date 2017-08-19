// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package osutil

import (
	"bytes"
	"net"
)

// ResolveInterfaceAddresses returns available addresses of the given network
// type for a given interface.
func ResolveInterfaceAddresses(network, nameOrMac string) []string {
	intf, err := net.InterfaceByName(nameOrMac)
	if err == nil {
		return interfaceAddresses(network, intf)
	}

	mac, err := net.ParseMAC(nameOrMac)
	if err != nil {
		return []string{nameOrMac}
	}

	intfs, err := net.Interfaces()
	if err != nil {
		return []string{nameOrMac}
	}

	for _, intf := range intfs {
		if bytes.Equal(intf.HardwareAddr, mac) {
			return interfaceAddresses(network, &intf)
		}
	}

	return []string{nameOrMac}
}

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

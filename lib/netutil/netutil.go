// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package netutil

import (
	"fmt"
	"net"
	"net/url"
	"os"

	"github.com/jackpal/gateway"
)

// Address constructs a URL from the given network and hostname.
func AddressURL(network, host string) string {
	u := url.URL{
		Scheme: network,
		Host:   host,
	}
	return u.String()
}

// Gateway returns the IP address of the default network gateway.
func Gateway() (ip net.IP, err error) {
	ip, err = gateway.DiscoverGateway()
	if err != nil {
		// Fails on Android 14+ due to permission denied error when reading
		// /proc/net/route. The wrapper may give a hint then because it is
		// able to discover the gateway from java code.
		if v := os.Getenv("FALLBACK_NET_GATEWAY_IPV4"); v != "" {
			ip = net.ParseIP(v)
			if ip == nil {
				return nil, fmt.Errorf("%q: invalid IP", v)
			}
			return ip, nil
		}
		return ip, err
	}
	return ip, nil
}

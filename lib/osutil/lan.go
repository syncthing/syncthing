// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil

import (
	"net"
)

func GetLans() ([]*net.IPNet, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
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

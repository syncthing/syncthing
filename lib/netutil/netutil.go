// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package netutil

import "net/url"

// Address constructs a URL from the given network and hostname.
func AddressURL(network, host string) string {
	u := url.URL{
		Scheme: network,
		Host:   host,
	}
	return u.String()
}

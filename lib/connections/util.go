// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package connections

import (
	"net"
	"net/url"
	"strconv"
	"strings"
)

func fixupPort(uri *url.URL, defaultPort int) *url.URL {
	copyURI := *uri

	host, port, err := net.SplitHostPort(uri.Host)
	if err != nil && strings.Contains(err.Error(), "missing port") {
		// addr is on the form "1.2.3.4"
		copyURI.Host = net.JoinHostPort(uri.Host, strconv.Itoa(defaultPort))
	} else if err == nil && port == "" {
		// addr is on the form "1.2.3.4:"
		copyURI.Host = net.JoinHostPort(host, strconv.Itoa(defaultPort))
	}

	return &copyURI
}

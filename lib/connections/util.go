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

	"github.com/syncthing/syncthing/lib/osutil"
)

func fixupPort(uri *url.URL, defaultPort int) *url.URL {
	copyURI := *uri

	host, port, err := net.SplitHostPort(uri.Host)
	if err != nil && strings.Contains(err.Error(), "missing port") {
		// addr is on the form "1.2.3.4" or "[fe80::1]"
		host = uri.Host
		if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
			// net.JoinHostPort will add the brackets again
			host = host[1 : len(host)-1]
		}
		copyURI.Host = net.JoinHostPort(host, strconv.Itoa(defaultPort))
	} else if err == nil && port == "" {
		// addr is on the form "1.2.3.4:" or "[fe80::1]:"
		copyURI.Host = net.JoinHostPort(host, strconv.Itoa(defaultPort))
	}

	return &copyURI
}

func getUrisForAllAdapters(listenUrl *url.URL) []*url.URL {
	nets, err := osutil.GetLans()
	if err != nil {
		// Ignore failure.
		return []*url.URL{listenUrl}
	}

	urls := make([]*url.URL, 0, len(nets)+1)
	urls = append(urls, listenUrl)

	port := listenUrl.Port()

	for _, network := range nets {
		if network.IP.IsGlobalUnicast() || network.IP.IsLinkLocalUnicast() {
			newUrl := *listenUrl
			newUrl.Host = net.JoinHostPort(network.IP.String(), port)
			urls = append(urls, &newUrl)
		}
	}
	return urls
}

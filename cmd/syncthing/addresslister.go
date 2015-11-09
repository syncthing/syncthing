// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"fmt"
	"net"
	"net/url"

	"github.com/syncthing/syncthing/lib/config"
)

type addressLister struct {
	upnpSvc *upnpSvc
	cfg     *config.Wrapper
}

func newAddressLister(upnpSvc *upnpSvc, cfg *config.Wrapper) *addressLister {
	return &addressLister{
		upnpSvc: upnpSvc,
		cfg:     cfg,
	}
}

// ExternalAddresses returns a list of addresses that are our best guess for
// where we are reachable from the outside. As a special case, we may return
// one or more addresses with an empty IP address (0.0.0.0 or ::) and just
// port number - this means that the outside address of a NAT gateway should
// be substituted.
func (e *addressLister) ExternalAddresses() []string {
	return e.addresses(false)
}

// AllAddresses returns a list of addresses that are our best guess for where
// we are reachable from the local network. Same conditions as
// ExternalAddresses, but private IPv4 addresses are included.
func (e *addressLister) AllAddresses() []string {
	return e.addresses(true)
}

func (e *addressLister) addresses(includePrivateIPV4 bool) []string {
	var addrs []string

	// Grab our listen addresses from the config. Unspecified ones are passed
	// on verbatim (to be interpreted by a global discovery server or local
	// discovery peer). Public addresses are passed on verbatim. Private
	// addresses are filtered.
	for _, addrStr := range e.cfg.Options().ListenAddress {
		addrURL, err := url.Parse(addrStr)
		if err != nil {
			l.Infoln("Listen address", addrStr, "is invalid:", err)
			continue
		}
		addr, err := net.ResolveTCPAddr("tcp", addrURL.Host)
		if err != nil {
			l.Infoln("Listen address", addrStr, "is invalid:", err)
			continue
		}

		if addr.IP == nil || addr.IP.IsUnspecified() {
			// Address like 0.0.0.0:22000 or [::]:22000 or :22000; include as is.
			addrs = append(addrs, tcpAddr(addr.String()))
		} else if isPublicIPv4(addr.IP) || isPublicIPv6(addr.IP) {
			// A public address; include as is.
			addrs = append(addrs, tcpAddr(addr.String()))
		} else if includePrivateIPV4 && addr.IP.To4().IsGlobalUnicast() {
			// A private IPv4 address.
			addrs = append(addrs, tcpAddr(addr.String()))
		}
	}

	// Get an external port mapping from the upnpSvc, if it has one. If so,
	// add it as another unspecified address.
	if e.upnpSvc != nil {
		if port := e.upnpSvc.ExternalPort(); port != 0 {
			addrs = append(addrs, fmt.Sprintf("tcp://:%d", port))
		}
	}

	return addrs
}

func isPublicIPv4(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil {
		// Not an IPv4 address (IPv6)
		return false
	}

	// IsGlobalUnicast below only checks that it's not link local or
	// multicast, and we want to exclude private (NAT:ed) addresses as well.
	rfc1918 := []net.IPNet{
		{IP: net.IP{10, 0, 0, 0}, Mask: net.IPMask{255, 0, 0, 0}},
		{IP: net.IP{172, 16, 0, 0}, Mask: net.IPMask{255, 240, 0, 0}},
		{IP: net.IP{192, 168, 0, 0}, Mask: net.IPMask{255, 255, 0, 0}},
	}
	for _, n := range rfc1918 {
		if n.Contains(ip) {
			return false
		}
	}

	return ip.IsGlobalUnicast()
}

func isPublicIPv6(ip net.IP) bool {
	if ip.To4() != nil {
		// Not an IPv6 address (IPv4)
		// (To16() returns a v6 mapped v4 address so can't be used to check
		// that it's an actual v6 address)
		return false
	}

	return ip.IsGlobalUnicast()
}

func tcpAddr(host string) string {
	u := url.URL{
		Scheme: "tcp",
		Host:   host,
	}
	return u.String()
}

// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// Adapted from https://github.com/jackpal/Taipei-Torrent/blob/dd88a8bfac6431c01d959ce3c745e74b8a911793/IGD.go
// Copyright (c) 2010 Jack Palevich (https://github.com/jackpal/Taipei-Torrent/blob/dd88a8bfac6431c01d959ce3c745e74b8a911793/LICENSE)

package upnp

import (
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/nat"
)

// An IGD is a UPnP InternetGatewayDevice.
type IGD struct {
	uuid           string
	friendlyName   string
	services       []IGDService
	url            *url.URL
	localIPAddress net.IP
}

func (n *IGD) ID() string {
	return n.uuid
}

func (n *IGD) FriendlyName() string {
	return n.friendlyName
}

// FriendlyIdentifier returns a friendly identifier (friendly name + IP
// address) for the IGD.
func (n *IGD) FriendlyIdentifier() string {
	return "'" + n.FriendlyName() + "' (" + strings.Split(n.URL().Host, ":")[0] + ")"
}

func (n *IGD) URL() *url.URL {
	return n.url
}

// AddPortMapping adds a port mapping to all relevant services on the
// specified InternetGatewayDevice. Port mapping will fail and return an error
// if action is fails for _any_ of the relevant services. For this reason, it
// is generally better to configure port mapping for each individual service
// instead.
func (n *IGD) AddPortMapping(protocol nat.Protocol, externalPort, internalPort int, description string, duration time.Duration) (int, error) {
	for _, service := range n.services {
		err := service.AddPortMapping(n.localIPAddress, protocol, externalPort, internalPort, description, duration)
		if err != nil {
			return externalPort, err
		}
	}
	return externalPort, nil
}

// DeletePortMapping deletes a port mapping from all relevant services on the
// specified InternetGatewayDevice. Port mapping will fail and return an error
// if action is fails for _any_ of the relevant services. For this reason, it
// is generally better to configure port mapping for each individual service
// instead.
func (n *IGD) DeletePortMapping(protocol nat.Protocol, externalPort int) error {
	for _, service := range n.services {
		err := service.DeletePortMapping(protocol, externalPort)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetExternalIPAddress returns the external IP address of the IGD, or an error
// if no service providing this feature exists.
func (n *IGD) GetExternalIPAddress() (ip net.IP, err error) {
	for _, service := range n.services {
		ip, err = service.GetExternalIPAddress()
		if err == nil {
			break
		}
	}
	return
}

// GetLocalIPAddress returns the IP address of the local network interface
// which is facing the IGD.
func (n *IGD) GetLocalIPAddress() net.IP {
	return n.localIPAddress
}

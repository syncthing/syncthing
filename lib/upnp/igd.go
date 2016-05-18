// Copyright (C) 2016 The Syncthing Authors.
//
// Adapted from https://github.com/jackpal/Taipei-Torrent/blob/dd88a8bfac6431c01d959ce3c745e74b8a911793/IGD.go
// Copyright (c) 2010 Jack Palevich (https://github.com/jackpal/Taipei-Torrent/blob/dd88a8bfac6431c01d959ce3c745e74b8a911793/LICENSE)
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
//    * Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
//    * Redistributions in binary form must reproduce the above
// copyright notice, this list of conditions and the following disclaimer
// in the documentation and/or other materials provided with the
// distribution.
//    * Neither the name of Google Inc. nor the names of its
// contributors may be used to endorse or promote products derived from
// this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
//

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
func (n *IGD) AddPortMapping(protocol nat.Protocol, internalPort, externalPort int, description string, duration time.Duration) (int, error) {
	for _, service := range n.services {
		err := service.AddPortMapping(n.localIPAddress, protocol, internalPort, externalPort, description, duration)
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

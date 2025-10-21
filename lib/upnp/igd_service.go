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
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/syncthing/syncthing/internal/slogutil"
	"github.com/syncthing/syncthing/lib/netutil"

	"github.com/syncthing/syncthing/lib/nat"
)

// An IGDService is a specific service provided by an IGD.
type IGDService struct {
	UUID      string
	Device    upnpDevice
	ServiceID string
	URL       string
	URN       string
	LocalIPv4 net.IP
	Interface *net.Interface
}

// AddPinhole adds an IPv6 pinhole in accordance to http://upnp.org/specs/gw/UPnP-gw-WANIPv6FirewallControl-v1-Service.pdf
// This is attempted for each IPv6 on the interface.
func (s *IGDService) AddPinhole(ctx context.Context, protocol nat.Protocol, intAddr nat.Address, duration time.Duration) ([]net.IP, error) {
	var returnErr error
	var successfulIPs []net.IP
	if s.Interface == nil {
		return nil, errors.New("no interface")
	}

	addrs, err := netutil.InterfaceAddrsByInterface(s.Interface)
	if err != nil {
		return nil, err
	}

	if !intAddr.IP.IsUnspecified() {
		// We have an explicit listener address. Check if that's on the interface
		// and pinhole it if so. It's not an error if not though, so don't return
		// an error if one doesn't occur.
		if intAddr.IP.To4() != nil {
			l.Debugf("Listener is IPv4. Not using gateway %s", s.ID())
			return nil, nil
		}
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				return nil, err
			}

			if ip.Equal(intAddr.IP) {
				err := s.tryAddPinholeForIP6(ctx, protocol, intAddr.Port, duration, intAddr.IP)
				if err != nil {
					return nil, err
				}
				return []net.IP{
					intAddr.IP,
				}, nil
			}

			l.Debugf("Listener IP %s not on interface for gateway %s", intAddr.IP, s.ID())
		}
		return nil, nil
	}

	// Otherwise, try to get a pinhole for all IPs, since we are listening on all
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			slog.WarnContext(ctx, "Couldn't parse interface address", slogutil.Address(addr), slogutil.Error(err))
			continue
		}

		// Note that IsGlobalUnicast allows ULAs.
		if ip.To4() != nil || !ip.IsGlobalUnicast() || ip.IsPrivate() {
			continue
		}

		if err := s.tryAddPinholeForIP6(ctx, protocol, intAddr.Port, duration, ip); err != nil {
			slog.WarnContext(ctx, "Couldn't add pinhole", slogutil.Address(ip), slog.Int("port", intAddr.Port), slog.Any("protocol", protocol), slogutil.Error(err))
			returnErr = err
		} else {
			successfulIPs = append(successfulIPs, ip)
		}
	}

	if len(successfulIPs) > 0 {
		// (Maybe partial) success, we added a pinhole for at least one GUA.
		return successfulIPs, nil
	} else {
		return nil, returnErr
	}
}

func (s *IGDService) tryAddPinholeForIP6(ctx context.Context, protocol nat.Protocol, port int, duration time.Duration, ip net.IP) error {
	var protoNumber int
	switch protocol {
	case nat.TCP:
		protoNumber = 6
	case nat.UDP:
		protoNumber = 17
	default:
		return errors.New("protocol not supported")
	}

	const template = `<u:AddPinhole xmlns:u="%s">
	<RemoteHost></RemoteHost>
	<RemotePort>0</RemotePort>
	<Protocol>%d</Protocol>
	<InternalPort>%d</InternalPort>
	<InternalClient>%s</InternalClient>
	<LeaseTime>%d</LeaseTime>
	</u:AddPinhole>`

	body := fmt.Sprintf(template, s.URN, protoNumber, port, ip, duration/time.Second)

	// IP should be a global unicast address, so we can use it as the source IP.
	// By the UPnP spec, the source address for unauthenticated clients should be
	// the same as the InternalAddress the pinhole is requested for.
	// Currently, WANIPv6FirewallProtocol is restricted to IPv6 gateways, so we can always set the IP.
	resp, err := soapRequestWithIP(ctx, s.URL, s.URN, "AddPinhole", body, &net.TCPAddr{IP: ip})
	if err != nil && resp != nil {
		var errResponse soapErrorResponse
		if unmarshalErr := xml.Unmarshal(resp, &errResponse); unmarshalErr != nil {
			// There is an error response that we cannot parse.
			return unmarshalErr
		}
		// There is a parsable UPnP error. Return that.
		return fmt.Errorf("UPnP error: %s (%d)", errResponse.ErrorDescription, errResponse.ErrorCode)
	} else if resp != nil {
		var succResponse soapAddPinholeResponse
		if unmarshalErr := xml.Unmarshal(resp, &succResponse); unmarshalErr != nil {
			// Ignore errors since this is only used for debug logging.
			l.Debugf("Failed to parse response from gateway %s: %s", s.ID(), unmarshalErr)
		} else {
			l.Debugf("UPnPv6: UID for pinhole on [%s]:%d/%s is %d on gateway %s", ip, port, protocol, succResponse.UniqueID, s.ID())
		}
	}
	// Either there was no error or an error not handled above (no response, e.g. network error).
	return err
}

// AddPortMapping adds a port mapping to the specified IGD service.
func (s *IGDService) AddPortMapping(ctx context.Context, protocol nat.Protocol, internalPort, externalPort int, description string, duration time.Duration) (int, error) {
	if s.LocalIPv4 == nil {
		return 0, errors.New("no local IPv4")
	}

	const template = `<u:AddPortMapping xmlns:u="%s">
		<NewRemoteHost></NewRemoteHost>
		<NewExternalPort>%d</NewExternalPort>
		<NewProtocol>%s</NewProtocol>
		<NewInternalPort>%d</NewInternalPort>
		<NewInternalClient>%s</NewInternalClient>
		<NewEnabled>1</NewEnabled>
		<NewPortMappingDescription>%s</NewPortMappingDescription>
		<NewLeaseDuration>%d</NewLeaseDuration>
		</u:AddPortMapping>`
	body := fmt.Sprintf(template, s.URN, externalPort, protocol, internalPort, s.LocalIPv4, description, duration/time.Second)

	response, err := soapRequestWithIP(ctx, s.URL, s.URN, "AddPortMapping", body, &net.TCPAddr{IP: s.LocalIPv4})
	if err != nil && duration > 0 {
		// Try to repair error code 725 - OnlyPermanentLeasesSupported
		var envelope soapErrorResponse
		if unmarshalErr := xml.Unmarshal(response, &envelope); unmarshalErr != nil {
			return externalPort, unmarshalErr
		}

		if envelope.ErrorCode == 725 {
			return s.AddPortMapping(ctx, protocol, internalPort, externalPort, description, 0)
		}

		err = fmt.Errorf("UPnP Error: %s (%d)", envelope.ErrorDescription, envelope.ErrorCode)
		l.Debugf("Couldn't add port mapping for %s (external port %d -> internal port %d/%s): %s", s.LocalIPv4, externalPort, internalPort, protocol, err)
	}

	return externalPort, err
}

// DeletePortMapping deletes a port mapping from the specified IGD service.
func (s *IGDService) DeletePortMapping(ctx context.Context, protocol nat.Protocol, externalPort int) error {
	const template = `<u:DeletePortMapping xmlns:u="%s">
	<NewRemoteHost></NewRemoteHost>
	<NewExternalPort>%d</NewExternalPort>
	<NewProtocol>%s</NewProtocol>
	</u:DeletePortMapping>`

	body := fmt.Sprintf(template, s.URN, externalPort, protocol)

	_, err := soapRequest(ctx, s.URL, s.URN, "DeletePortMapping", body)
	return err
}

// GetExternalIPv4Address queries the IGD service for its external IP address.
// Returns nil if the external IP address is invalid or undefined, along with
// any relevant errors
func (s *IGDService) GetExternalIPv4Address(ctx context.Context) (net.IP, error) {
	const template = `<u:GetExternalIPAddress xmlns:u="%s" />`

	body := fmt.Sprintf(template, s.URN)
	response, err := soapRequest(ctx, s.URL, s.URN, "GetExternalIPAddress", body)
	if err != nil {
		return nil, err
	}

	var envelope soapGetExternalIPAddressResponseEnvelope
	if err := xml.Unmarshal(response, &envelope); err != nil {
		return nil, err
	}

	result := net.ParseIP(envelope.Body.GetExternalIPAddressResponse.NewExternalIPAddress)

	return result, nil
}

// GetLocalIPv4Address returns local IP address used to contact this service
func (s *IGDService) GetLocalIPv4Address() net.IP {
	return s.LocalIPv4
}

// SupportsIPVersion checks whether this is a WANIPv6FirewallControl device,
// in which case pinholing instead of port mapping should be done
func (s *IGDService) SupportsIPVersion(version nat.IPVersion) bool {
	switch version {
	case nat.IPvAny:
		return true
	case nat.IPv6Only:
		return s.URN == urnWANIPv6FirewallControlV1
	case nat.IPv4Only:
		return s.URN != urnWANIPv6FirewallControlV1
	}

	return true
}

// ID returns a unique ID for the service
func (s *IGDService) ID() string {
	return s.UUID + "/" + s.Device.FriendlyName + "/" + s.ServiceID + "/" + s.URN + "/" + s.URL
}

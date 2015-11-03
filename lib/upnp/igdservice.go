// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package upnp

import (
	"encoding/xml"
	"fmt"
	"net"

	"github.com/syncthing/syncthing/lib/nat"
)

// An IGDService is a specific service provided by an IGD.
type IGDService struct {
	ID  string
	URL string
	URN string
}

// AddPortMapping adds a port mapping to the specified IGD service.
func (s *IGDService) AddPortMapping(localIPAddress net.IP, protocol nat.Protocol, externalPort, internalPort int, description string, timeout int) error {
	tpl := `<u:AddPortMapping xmlns:u="%s">
	<NewRemoteHost></NewRemoteHost>
	<NewExternalPort>%d</NewExternalPort>
	<NewProtocol>%s</NewProtocol>
	<NewInternalPort>%d</NewInternalPort>
	<NewInternalClient>%s</NewInternalClient>
	<NewEnabled>1</NewEnabled>
	<NewPortMappingDescription>%s</NewPortMappingDescription>
	<NewLeaseDuration>%d</NewLeaseDuration>
	</u:AddPortMapping>`
	ipStr := localIPAddress.String()
	if localIPAddress == nil || localIPAddress.IsUnspecified() {
		ipStr = ""
	}
	body := fmt.Sprintf(tpl, s.URN, externalPort, protocol, internalPort, ipStr, description, timeout)

	response, err := soapRequest(s.URL, s.URN, "AddPortMapping", body)
	if err != nil && timeout > 0 {
		// Try to repair error code 725 - OnlyPermanentLeasesSupported
		envelope := &soapErrorResponse{}
		if unmarshalErr := xml.Unmarshal(response, envelope); unmarshalErr != nil {
			return unmarshalErr
		}
		if envelope.ErrorCode == 725 {
			return s.AddPortMapping(localIPAddress, protocol, externalPort, internalPort, description, 0)
		}
	}

	return err
}

// DeletePortMapping deletes a port mapping from the specified IGD service.
func (s *IGDService) DeletePortMapping(protocol nat.Protocol, externalPort int) error {
	tpl := `<u:DeletePortMapping xmlns:u="%s">
	<NewRemoteHost></NewRemoteHost>
	<NewExternalPort>%d</NewExternalPort>
	<NewProtocol>%s</NewProtocol>
	</u:DeletePortMapping>`
	body := fmt.Sprintf(tpl, s.URN, externalPort, protocol)

	_, err := soapRequest(s.URL, s.URN, "DeletePortMapping", body)

	if err != nil {
		return err
	}

	return nil
}

// GetExternalIPAddress queries the IGD service for its external IP address.
// Returns nil if the external IP address is invalid or undefined, along with
// any relevant errors
func (s *IGDService) GetExternalIPAddress() (net.IP, error) {
	tpl := `<u:GetExternalIPAddress xmlns:u="%s" />`

	body := fmt.Sprintf(tpl, s.URN)

	response, err := soapRequest(s.URL, s.URN, "GetExternalIPAddress", body)

	if err != nil {
		return nil, err
	}

	envelope := &soapGetExternalIPAddressResponseEnvelope{}
	err = xml.Unmarshal(response, envelope)
	if err != nil {
		return nil, err
	}

	result := net.ParseIP(envelope.Body.GetExternalIPAddressResponse.NewExternalIPAddress)

	return result, nil
}

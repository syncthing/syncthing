// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package upnp

import (
	"encoding/xml"
	"net/url"
	"testing"
)

func TestExternalIPParsing(t *testing.T) {
	soapResponse := []byte(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
		<s:Body>
			<u:GetExternalIPAddressResponse xmlns:u="urn:schemas-upnp-org:service:WANIPConnection:1">
			<NewExternalIPAddress>1.2.3.4</NewExternalIPAddress>
			</u:GetExternalIPAddressResponse>
		</s:Body>
		</s:Envelope>`)

	envelope := &soapGetExternalIPAddressResponseEnvelope{}
	err := xml.Unmarshal(soapResponse, envelope)
	if err != nil {
		t.Error(err)
	}

	if envelope.Body.GetExternalIPAddressResponse.NewExternalIPAddress != "1.2.3.4" {
		t.Error("Parse of SOAP request failed.")
	}
}

func TestSoapFaultParsing(t *testing.T) {
	soapResponse := []byte(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
		<s:Body>
			<s:Fault>
				<faultcode>s:Client</faultcode>
				<faultstring>UPnPError</faultstring>
				<detail>
					<UPnPError xmlns="urn:schemas-upnp-org:control-1-0">
					<errorCode>725</errorCode>
					<errorDescription>OnlyPermanentLeasesSupported</errorDescription></UPnPError>
				</detail>
			</s:Fault>
		</s:Body>
		</s:Envelope>`)

	envelope := &soapErrorResponse{}
	err := xml.Unmarshal(soapResponse, envelope)
	if err != nil {
		t.Error(err)
	}

	if envelope.ErrorCode != 725 {
		t.Error("Parse of SOAP request failed.", envelope)
	}
}

func TestControlURLParsing(t *testing.T) {
	rootURL := "http://192.168.243.1:80/igd.xml"

	u, _ := url.Parse(rootURL)
	subject := "/upnp?control=WANCommonIFC1"
	expected := "http://192.168.243.1:80/upnp?control=WANCommonIFC1"
	replaceRawPath(u, subject)

	if u.String() != expected {
		t.Error("URL normalization of", subject, "failed; expected", expected, "got", u.String())
	}

	u, _ = url.Parse(rootURL)
	subject = "http://192.168.243.1:80/upnp?control=WANCommonIFC1"
	expected = "http://192.168.243.1:80/upnp?control=WANCommonIFC1"
	replaceRawPath(u, subject)

	if u.String() != expected {
		t.Error("URL normalization of", subject, "failed; expected", expected, "got", u.String())
	}
}

// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package upnp

import (
	"encoding/xml"
	"net/url"
	"testing"
)

func TestExternalIPParsing(t *testing.T) {
	soapResponse :=
		[]byte(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
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

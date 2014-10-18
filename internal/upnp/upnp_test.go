// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
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
	"testing"
)

func TestExternalIPParsing(t *testing.T) {
	soap_response :=
		[]byte(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
		<s:Body>
			<u:GetExternalIPAddressResponse xmlns:u="urn:schemas-upnp-org:service:WANIPConnection:1">
			<NewExternalIPAddress>1.2.3.4</NewExternalIPAddress>
			</u:GetExternalIPAddressResponse>
		</s:Body>
		</s:Envelope>`)

	envelope := &soapGetExternalIPAddressResponseEnvelope{}
	err := xml.Unmarshal(soap_response, envelope)
	if err != nil {
		t.Error(err)
	}

	if envelope.Body.GetExternalIPAddressResponse.NewExternalIPAddress != "1.2.3.4" {
		t.Error("Parse of SOAP request failed.")
	}
}

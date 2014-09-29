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
	"os"
	"testing"
)

func TestGetTechnicolorRootURL(t *testing.T) {
	r, _ := os.Open("testdata/technicolor.xml")
	_, action, err := getServiceURLReader("http://localhost:1234/", r)
	if err != nil {
		t.Fatal(err)
	}
	if action != "urn:schemas-upnp-org:service:WANPPPConnection:1" {
		t.Error("Unexpected action", action)
	}
}

// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

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

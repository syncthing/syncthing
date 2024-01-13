// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package geoip

import (
	"net"
	"os"
	"testing"
)

func TestDownloadAndOpen(t *testing.T) {
	license := os.Getenv("UR_GEOIP_LICENSE")
	if license == "" {
		t.Skip("No license key set")
	}

	p := NewGeoLite2CityProvider(license, t.TempDir())
	_, err := p.City(net.ParseIP("8.8.8.8"))
	if err != nil {
		t.Fatal(err)
	}
}

// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package geoip

import (
	"context"
	"net"
	"os"
	"strconv"
	"testing"
)

func TestDownloadAndOpen(t *testing.T) {
	acctID, _ := strconv.Atoi(os.Getenv("GEOIP_ACCOUNT_ID"))
	if acctID == 0 {
		t.Skip("No account ID set")
	}
	license := os.Getenv("GEOIP_LICENSE_KEY")
	if license == "" {
		t.Skip("No license key set")
	}

	p, err := NewGeoLite2CityProvider(context.Background(), acctID, license, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	_, err = p.City(net.ParseIP("8.8.8.8"))
	if err != nil {
		t.Fatal(err)
	}
}

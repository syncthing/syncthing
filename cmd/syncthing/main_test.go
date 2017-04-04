// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"testing"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestShortIDCheck(t *testing.T) {
	cfg := config.Wrap("/tmp/test", config.Configuration{
		Devices: []config.DeviceConfiguration{
			{DeviceID: protocol.DeviceID{8, 16, 24, 32, 40, 48, 56, 0, 0}},
			{DeviceID: protocol.DeviceID{8, 16, 24, 32, 40, 48, 56, 1, 1}}, // first 56 bits same, differ in the first 64 bits
		},
	})

	if err := checkShortIDs(cfg); err != nil {
		t.Error("Unexpected error:", err)
	}

	cfg = config.Wrap("/tmp/test", config.Configuration{
		Devices: []config.DeviceConfiguration{
			{DeviceID: protocol.DeviceID{8, 16, 24, 32, 40, 48, 56, 64, 0}},
			{DeviceID: protocol.DeviceID{8, 16, 24, 32, 40, 48, 56, 64, 1}}, // first 64 bits same
		},
	})

	if err := checkShortIDs(cfg); err == nil {
		t.Error("Should have gotten an error")
	}
}

func TestAllowedVersions(t *testing.T) {
	testcases := []struct {
		ver     string
		allowed bool
	}{
		{"v0.13.0", true},
		{"v0.12.11+22-gabcdef0", true},
		{"v0.13.0-beta0", true},
		{"v0.13.0-beta47", true},
		{"v0.13.0-beta47+1-gabcdef0", true},
		{"v0.13.0-beta.0", true},
		{"v0.13.0-beta.47", true},
		{"v0.13.0-beta.0+1-gabcdef0", true},
		{"v0.13.0-beta.47+1-gabcdef0", true},
		{"v0.13.0-some-weird-but-allowed-tag", true},
		{"v0.13.0-allowed.to.do.this", true},
		{"v0.13.0+not.allowed.to.do.this", false},
	}

	for i, c := range testcases {
		if allowed := allowedVersionExp.MatchString(c.ver); allowed != c.allowed {
			t.Errorf("%d: incorrect result %v != %v for %q", i, allowed, c.allowed, c.ver)
		}
	}
}

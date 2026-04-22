// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import "testing"

func TestMigrateCrashReporting(t *testing.T) {
	// When migrating from pre-crash-reporting configs, crash reporting is
	// enabled if global discovery is enabled or if usage reporting is
	// enabled (not just undecided).
	cases := []struct {
		opts    OptionsConfiguration
		enabled bool
	}{
		{opts: OptionsConfiguration{URAccepted: 0, GlobalAnnEnabled: true}, enabled: true},
		{opts: OptionsConfiguration{URAccepted: -1, GlobalAnnEnabled: true}, enabled: true},
		{opts: OptionsConfiguration{URAccepted: 1, GlobalAnnEnabled: true}, enabled: true},
		{opts: OptionsConfiguration{URAccepted: 0, GlobalAnnEnabled: false}, enabled: false},
		{opts: OptionsConfiguration{URAccepted: -1, GlobalAnnEnabled: false}, enabled: false},
		{opts: OptionsConfiguration{URAccepted: 1, GlobalAnnEnabled: false}, enabled: true},
	}

	for i, tc := range cases {
		cfg := Configuration{Version: 28, Options: tc.opts}
		migrationsMut.Lock()
		migrations.apply(&cfg)
		migrationsMut.Unlock()
		if cfg.Options.CREnabled != tc.enabled {
			t.Errorf("%d: unexpected result, CREnabled: %v != %v", i, cfg.Options.CREnabled, tc.enabled)
		}
	}
}

func TestMigrateReconnectInterval(t *testing.T) {
	cases := []struct {
		oldInterval         int
		expectedNewInterval int
	}{
		{oldInterval: 60, expectedNewInterval: 20},
		{oldInterval: 120, expectedNewInterval: 40},
		{oldInterval: 25, expectedNewInterval: 10},
		{oldInterval: 5, expectedNewInterval: 5},
	}

	for i, tc := range cases {
		cfg := Configuration{Version: 51, Options: OptionsConfiguration{ReconnectIntervalS: tc.oldInterval}}
		migrationsMut.Lock()
		migrations.apply(&cfg)
		migrationsMut.Unlock()
		if cfg.Options.ReconnectIntervalS != tc.expectedNewInterval {
			t.Errorf("%d: unexpected result, ReconnectIntervalS: %v != %v", i, cfg.Options.ReconnectIntervalS, tc.expectedNewInterval)
		}
	}
}

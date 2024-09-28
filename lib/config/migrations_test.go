// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

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

func TestMigration38(t *testing.T) {
	{
		cfg := Configuration{
			Options: OptionsConfiguration{
				UnackedNotificationIDs: []string{"foo", "authenticationUserAndPassword", "bar"},
			},
			GUI: GUIConfiguration{
				RawUseTLS: false,
			},
		}
		migrateToConfigV38(&cfg)
		if !cmp.Equal(cfg.Options.UnackedNotificationIDs, []string{"foo", "guiAuthentication", "bar"}) {
			t.Error("Expected notification \"authenticationUserAndPassword\" to be renamed to \"guiAuthentication\"")
		}
		if cfg.GUI.WebauthnRpId != "localhost" {
			t.Error("Expected GUI.WebauthnRpId to be set to \"localhost\"")
		}
		if !cmp.Equal(cfg.GUI.WebauthnOrigins, []string{"https://localhost:8384", "http://localhost:8384"}) {
			t.Error("Expected GUI.WebauthnOrigins to be set to default values")
		}
	}

	{
		cfg := Configuration{
			GUI: GUIConfiguration{
				RawAddress: "127.0.0.1:8888",
				RawUseTLS:  true,
			},
		}
		migrateToConfigV38(&cfg)
		if !cmp.Equal(cfg.GUI.WebauthnOrigins, []string{"https://localhost:8888"}) {
			t.Error("Expected GUI.WebauthnOrigins to be set to default values with port 8888 and HTTPS only")
		}
	}

	{
		cfg := Configuration{
			GUI: GUIConfiguration{
				RawAddress: "127.0.0.1:443",
				RawUseTLS:  true,
			},
		}
		migrateToConfigV38(&cfg)
		if !cmp.Equal(cfg.GUI.WebauthnOrigins, []string{"https://localhost"}) {
			t.Error("Expected GUI.WebauthnOrigins to be set to default values with implicit HTTPS port and HTTPS only")
		}
	}

	{
		cfg := Configuration{
			GUI: GUIConfiguration{
				RawAddress: "127.0.0.1:80",
				RawUseTLS:  false,
			},
		}
		migrateToConfigV38(&cfg)
		if !cmp.Equal(cfg.GUI.WebauthnOrigins, []string{"https://localhost:80", "http://localhost"}) {
			t.Error("Expected GUI.WebauthnOrigins to be set to default values with implicit HTTP port")
		}
	}
}

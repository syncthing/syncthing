// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"os"
	"testing"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/syncthing"
)

func TestConfigHealthCommand(t *testing.T) {
	t.Run("valid config passes", func(t *testing.T) {
		setConfigBaseDir(t, t.TempDir())
		cfgFile := locations.Get(locations.ConfigFile)
		if err := config.Wrap(cfgFile, config.New(protocol.EmptyDeviceID), protocol.EmptyDeviceID, events.NoopLogger).Save(); err != nil {
			t.Fatal(err)
		}
		if err := (&configHealthCommand{}).Run(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing config fails", func(t *testing.T) {
		setConfigBaseDir(t, t.TempDir())
		if err := (&configHealthCommand{}).Run(); err == nil {
			t.Fatal("expected an error for a missing config, got nil")
		}
	})

	t.Run("malformed config fails", func(t *testing.T) {
		setConfigBaseDir(t, t.TempDir())
		if err := os.WriteFile(locations.Get(locations.ConfigFile), []byte("<configuration>"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := (&configHealthCommand{}).Run(); err == nil {
			t.Fatal("expected an error for a malformed config, got nil")
		}
	})
}

func TestKeyHealthCommand(t *testing.T) {
	t.Run("valid key pair passes", func(t *testing.T) {
		setConfigBaseDir(t, t.TempDir())
		if _, err := syncthing.GenerateCertificate(locations.Get(locations.CertFile), locations.Get(locations.KeyFile)); err != nil {
			t.Fatal(err)
		}
		if err := (&keyHealthCommand{}).Run(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing key pair fails", func(t *testing.T) {
		setConfigBaseDir(t, t.TempDir())
		if err := (&keyHealthCommand{}).Run(); err == nil {
			t.Fatal("expected an error for a missing key pair, got nil")
		}
	})
}

func setConfigBaseDir(t *testing.T, dir string) {
	t.Helper()
	orig := locations.GetBaseDir(locations.ConfigBaseDir)
	if err := locations.SetBaseDir(locations.ConfigBaseDir, dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := locations.SetBaseDir(locations.ConfigBaseDir, orig); err != nil {
			t.Fatal(err)
		}
	})
}

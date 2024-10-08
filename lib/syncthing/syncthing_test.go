// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package syncthing

import (
	"os"
	"slices"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/structutil"
	"github.com/syncthing/syncthing/lib/svcutil"
	"github.com/syncthing/syncthing/lib/tlsutil"
)

func tempCfgFilename(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "syncthing-testConfig-")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	return f.Name()
}

func TestShortIDCheck(t *testing.T) {
	cfg := config.Wrap(tempCfgFilename(t), config.Configuration{
		Devices: []config.DeviceConfiguration{
			{DeviceID: protocol.DeviceID{8, 16, 24, 32, 40, 48, 56, 0, 0}},
			{DeviceID: protocol.DeviceID{8, 16, 24, 32, 40, 48, 56, 1, 1}}, // first 56 bits same, differ in the first 64 bits
		},
	}, protocol.LocalDeviceID, events.NoopLogger)
	defer os.Remove(cfg.ConfigPath())

	if err := checkShortIDs(cfg); err != nil {
		t.Error("Unexpected error:", err)
	}

	cfg = config.Wrap("/tmp/test", config.Configuration{
		Devices: []config.DeviceConfiguration{
			{DeviceID: protocol.DeviceID{8, 16, 24, 32, 40, 48, 56, 64, 0}},
			{DeviceID: protocol.DeviceID{8, 16, 24, 32, 40, 48, 56, 64, 1}}, // first 64 bits same
		},
	}, protocol.LocalDeviceID, events.NoopLogger)

	if err := checkShortIDs(cfg); err == nil {
		t.Error("Should have gotten an error")
	}
}

func TestStartupFail(t *testing.T) {
	cert, err := tlsutil.NewCertificateInMemory("syncthing", 365)
	if err != nil {
		t.Fatal(err)
	}
	id := protocol.NewDeviceID(cert.Certificate[0])
	conflID := protocol.DeviceID{}
	copy(conflID[:8], id[:8])

	cfg := config.Wrap(tempCfgFilename(t), config.Configuration{
		Devices: []config.DeviceConfiguration{
			{DeviceID: id},
			{DeviceID: conflID},
		},
	}, protocol.LocalDeviceID, events.NoopLogger)
	defer os.Remove(cfg.ConfigPath())

	db := backend.OpenMemory()
	app, err := New(cfg, db, events.NoopLogger, cert, Options{})
	if err != nil {
		t.Fatal(err)
	}
	startErr := app.Start()
	if startErr == nil {
		t.Fatal("Expected an error from Start, got nil")
	}

	done := make(chan struct{})
	var waitE svcutil.ExitStatus
	go func() {
		waitE = app.Wait()
		close(done)
	}()

	select {
	case <-time.After(time.Second):
		t.Fatal("Wait did not return within 1s")
	case <-done:
	}

	if waitE != svcutil.ExitError {
		t.Errorf("Got exit status %v, expected %v", waitE, svcutil.ExitError)
	}

	if err = app.Error(); err != startErr {
		t.Errorf(`Got different errors "%v" from Start and "%v" from Error`, startErr, err)
	}

	if trans, err := db.NewReadTransaction(); err == nil {
		t.Error("Expected error due to db being closed, got nil")
		trans.Release()
	} else if !backend.IsClosed(err) {
		t.Error("Expected error due to db being closed, got", err)
	}
}

type defaultConfigCase struct {
	defaultFolder      bool
	portProbing        bool
	portBusy           bool
	guiAddressEnv      string
	guiAddressExpected string
}

func TestDefaultConfig(t *testing.T) {
	cases := []defaultConfigCase{
		// Hard-coded minimal default, no adjustments
		{false, false, false, "", "127.0.0.1:8384"},
		// Busy port should not matter without port probing
		{false, false, true, "", "127.0.0.1:8384"},
		// Add a default folder
		{true, false, false, "", "127.0.0.1:8384"},
		// Override GUI address without port probing
		{false, false, false, "0.0.0.0:8385", "0.0.0.0:8385"},
		// No override, with port probing
		{false, true, false, "", "127.0.0.1:8384"},
		// No override, with unsuccessful port probing
		{false, true, true, "", "127.0.0.1:8384"},
		// Override GUI address with port probing
		{false, true, false, "0.0.0.0:8385", "0.0.0.0:8385"},
		// Override GUI address with unsuccessful port probing
		{false, true, true, "0.0.0.0:8385", "0.0.0.0:8385"},
	}

	for _, c := range cases {
		subtestDefaultConfig(t, c)
	}
}

func subtestDefaultConfig(t *testing.T, c defaultConfigCase) {
	t.Logf("%v case: %+v", t.Name(), c)

	if c.guiAddressEnv != "" {
		os.Setenv("STGUIADDRESS", c.guiAddressEnv)
	} else {
		os.Unsetenv("STGUIADDRESS")
	}

	oldGetFreePort := config.GetFreePort
	config.GetFreePort = func(host string, ports ...int) (int, error) {
		if !c.portBusy {
			t.Logf(`Simulating non-blocked port %d on "%v"`, ports[0], host)
			return ports[0], nil
		}
		freePort := slices.Max(ports) + 1
		t.Logf(`Simulating blocked ports %v (using %d) on "%v"`, ports, freePort, host)
		return freePort, nil
	}
	defer func() {
		config.GetFreePort = oldGetFreePort
	}()

	if c.portBusy || c.portProbing {
		address := c.guiAddressEnv
		if address == "" {
			defaultGuiCfg := config.GUIConfiguration{}
			structutil.SetDefaults(&defaultGuiCfg)
			address = defaultGuiCfg.RawAddress
		}
	}

	cfg, err := DefaultConfig(tempCfgFilename(t), protocol.LocalDeviceID, events.NoopLogger, !c.defaultFolder, !c.portProbing)
	defer os.Remove(cfg.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}

	if c.defaultFolder {
		if len(cfg.FolderList()) != 1 {
			t.Error("Expected exactly one default folder in fresh configuration")
		} else if _, ok := cfg.Folder("default"); !ok {
			t.Error(`Folder "default" not found in fresh configuration`)
		}
	} else if len(cfg.FolderList()) != 0 {
		t.Error("Unexpected default folder in fresh configuration")
	}

	if c.portProbing && c.portBusy {
		// Replacement port is random, can only check it's not the same as requested
		if cfg.GUI().RawAddress == c.guiAddressExpected {
			t.Errorf(`Port probing chose blocked raw address "%v"`, cfg.GUI().RawAddress)
		}
	} else if cfg.GUI().RawAddress != c.guiAddressExpected {
		t.Errorf(`Expected raw address "%v" without port probing, got "%v"`, c.guiAddressExpected, cfg.GUI().RawAddress)
	}
}

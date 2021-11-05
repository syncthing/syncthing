// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package syncthing

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/svcutil"
	"github.com/syncthing/syncthing/lib/tlsutil"
)

func tempCfgFilename(t *testing.T) string {
	t.Helper()
	f, err := ioutil.TempFile("", "syncthing-testConfig-")
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
	cert, err := tlsutil.NewCertificate("", "", "syncthing", 365)
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

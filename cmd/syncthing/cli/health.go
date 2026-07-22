// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"crypto/tls"
	"fmt"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/protocol"
)

type configHealthCommand struct{}

func (*configHealthCommand) Run() error {
	cfgFile := locations.Get(locations.ConfigFile)
	if _, _, err := config.Load(cfgFile, protocol.EmptyDeviceID, events.NoopLogger); err != nil {
		return fmt.Errorf("checking configuration %q: %w", cfgFile, err)
	}
	return prettyPrintJSON(struct {
		Config string `json:"config"`
		Status string `json:"status"`
	}{cfgFile, "ok"})
}

type keyHealthCommand struct{}

func (*keyHealthCommand) Run() error {
	certFile, keyFile := locations.Get(locations.CertFile), locations.Get(locations.KeyFile)
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("checking certificate and key: %w", err)
	}
	return prettyPrintJSON(struct {
		Cert     string `json:"cert"`
		Key      string `json:"key"`
		DeviceID string `json:"deviceID"`
		Status   string `json:"status"`
	}{certFile, keyFile, protocol.NewDeviceID(cert.Certificate[0]).String(), "ok"})
}

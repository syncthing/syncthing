// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package guiclient provides shared helpers for connecting to the Syncthing
// REST API, used by both the CLI and TUI commands.
package guiclient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/protocol"
)

// LoadGUIConfig reads the GUI address and API key from the local Syncthing
// configuration file. This is the auto-discovery path used when no explicit
// --gui-address / --gui-apikey flags are provided.
func LoadGUIConfig() (config.GUIConfiguration, error) {
	cert, err := tls.LoadX509KeyPair(
		locations.Get(locations.CertFile),
		locations.Get(locations.KeyFile),
	)
	if err != nil {
		return config.GUIConfiguration{}, fmt.Errorf("reading device ID: %w", err)
	}

	myID := protocol.NewDeviceID(cert.Certificate[0])

	cfg, _, err := config.Load(locations.Get(locations.ConfigFile), myID, events.NoopLogger)
	if err != nil {
		return config.GUIConfiguration{}, fmt.Errorf("loading config: %w", err)
	}

	guiCfg := cfg.GUI()
	if guiCfg.Address() == "" {
		return config.GUIConfiguration{}, errors.New("could not find GUI Address")
	}
	if guiCfg.APIKey == "" {
		return config.GUIConfiguration{}, errors.New("could not find GUI API key")
	}
	return guiCfg, nil
}

// NewHTTPClient creates an *http.Client configured to connect to the
// Syncthing GUI, including TLS and unix socket support.
func NewHTTPClient(guiCfg config.GUIConfiguration) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial(guiCfg.Network(), guiCfg.Address())
			},
		},
	}
}

// BaseURL returns the HTTP base URL for the given GUI configuration,
// handling both TCP and unix socket addresses.
func BaseURL(guiCfg config.GUIConfiguration) string {
	if guiCfg.Network() == "unix" {
		return "http://unix/"
	}
	url := guiCfg.URL()
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	return url
}

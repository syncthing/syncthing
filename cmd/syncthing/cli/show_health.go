// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/protocol"
)

// checkConfigHealth verifies that the configuration file on disk is
// well-formed, without requiring a running Syncthing instance.
func checkConfigHealth() error {
	path := locations.Get(locations.ConfigFile)
	if _, _, err := config.Load(path, protocol.EmptyDeviceID, events.NoopLogger); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// checkKeyHealth verifies that the device certificate and key on disk exist,
// are parseable, and match each other, without requiring a running
// Syncthing instance.
func checkKeyHealth() error {
	certFile := locations.Get(locations.CertFile)
	keyFile := locations.Get(locations.KeyFile)

	if _, err := os.Stat(certFile); err != nil {
		return fmt.Errorf("certificate file: %w", err)
	}
	if _, err := os.Stat(keyFile); err != nil {
		return fmt.Errorf("key file: %w", err)
	}

	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return fmt.Errorf("reading certificate file: %w", err)
	}
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return fmt.Errorf("%s: failed to decode PEM data", certFile)
	}
	if _, err := x509.ParseCertificate(certBlock.Bytes); err != nil {
		return fmt.Errorf("parsing certificate: %w", err)
	}

	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return fmt.Errorf("reading key file: %w", err)
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return fmt.Errorf("%s: failed to decode PEM data", keyFile)
	}
	if _, err := parsePrivateKey(keyBlock); err != nil {
		return fmt.Errorf("parsing key: %w", err)
	}

	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return fmt.Errorf("certificate and key do not match: %w", err)
	}

	return nil
}

func parsePrivateKey(block *pem.Block) (any, error) {
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		return x509.ParseECPrivateKey(block.Bytes)
	default:
		return x509.ParsePKCS8PrivateKey(block.Bytes)
	}
}

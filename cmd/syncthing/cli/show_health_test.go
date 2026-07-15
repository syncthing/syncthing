// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/tlsutil"
)

func setBaseDirs(t *testing.T, dir string) {
	t.Helper()
	origConfig := locations.GetBaseDir(locations.ConfigBaseDir)
	origData := locations.GetBaseDir(locations.DataBaseDir)
	if err := locations.SetBaseDir(locations.ConfigBaseDir, dir); err != nil {
		t.Fatal(err)
	}
	if err := locations.SetBaseDir(locations.DataBaseDir, dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = locations.SetBaseDir(locations.ConfigBaseDir, origConfig)
		_ = locations.SetBaseDir(locations.DataBaseDir, origData)
	})
}

func TestCheckConfigHealthMissing(t *testing.T) {
	setBaseDirs(t, t.TempDir())

	if err := checkConfigHealth(); err == nil {
		t.Fatal("expected an error for a missing config file")
	}
}

func TestCheckConfigHealthInvalid(t *testing.T) {
	dir := t.TempDir()
	setBaseDirs(t, dir)

	if err := os.WriteFile(locations.Get(locations.ConfigFile), []byte("not valid xml"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := checkConfigHealth(); err == nil {
		t.Fatal("expected an error for a malformed config file")
	}
}

func TestCheckConfigHealthValid(t *testing.T) {
	dir := t.TempDir()
	setBaseDirs(t, dir)

	const validConfig = `<configuration version="37"></configuration>`
	if err := os.WriteFile(locations.Get(locations.ConfigFile), []byte(validConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := checkConfigHealth(); err != nil {
		t.Fatalf("expected no error for a valid config file, got: %v", err)
	}
}

func TestCheckKeyHealthMissing(t *testing.T) {
	setBaseDirs(t, t.TempDir())

	if err := checkKeyHealth(); err == nil {
		t.Fatal("expected an error for missing cert/key files")
	}
}

func TestCheckKeyHealthInvalid(t *testing.T) {
	dir := t.TempDir()
	setBaseDirs(t, dir)

	if err := os.WriteFile(locations.Get(locations.CertFile), []byte("not a cert"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(locations.Get(locations.KeyFile), []byte("not a key"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := checkKeyHealth(); err == nil {
		t.Fatal("expected an error for malformed cert/key files")
	}
}

func TestCheckKeyHealthMismatched(t *testing.T) {
	dir := t.TempDir()
	setBaseDirs(t, dir)

	certFile := locations.Get(locations.CertFile)
	keyFile := locations.Get(locations.KeyFile)
	if _, err := tlsutil.NewCertificate(certFile, keyFile, "syncthing", 1, true); err != nil {
		t.Fatal(err)
	}

	// Overwrite the key with an unrelated one, so the cert and key no longer match.
	otherKeyFile := filepath.Join(dir, "other-key.pem")
	otherCertFile := filepath.Join(dir, "other-cert.pem")
	if _, err := tlsutil.NewCertificate(otherCertFile, otherKeyFile, "other", 1, true); err != nil {
		t.Fatal(err)
	}
	keyPEM, err := os.ReadFile(otherKeyFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := checkKeyHealth(); err == nil {
		t.Fatal("expected an error for mismatched cert/key")
	}
}

func TestCheckKeyHealthValid(t *testing.T) {
	dir := t.TempDir()
	setBaseDirs(t, dir)

	certFile := locations.Get(locations.CertFile)
	keyFile := locations.Get(locations.KeyFile)
	if _, err := tlsutil.NewCertificate(certFile, keyFile, "syncthing", 1, true); err != nil {
		t.Fatal(err)
	}

	if err := checkKeyHealth(); err != nil {
		t.Fatalf("expected no error for a valid cert/key pair, got: %v", err)
	}
}

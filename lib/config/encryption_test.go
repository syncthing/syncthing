// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestEncryption(t *testing.T) {
	str := "testdata to obfuscate"
	enc := encryptString(str)
	if strings.Contains(enc, str) {
		t.Error("encryption should change the data")
	}
	t.Log(enc)

	res, ok := decryptString(enc)
	if !ok || res != str {
		t.Log(res, ok)
		t.Error("decryption should restore the data")
	}
}

func TestEncryptionInSerialization(t *testing.T) {
	// Verifies that secrets are encrypted in XML (on disk) but not in JSON
	// (API).

	const original = "plaintext string"
	var cfg Configuration
	cfg.GUI.APIKey.Set(original)

	var buf bytes.Buffer
	cfg.WriteXML(&buf)
	if strings.Contains(buf.String(), original) {
		t.Error("XML serialization should not contain plaintext string")
	}

	bs, _ := json.Marshal(cfg)
	if !strings.Contains(string(bs), original) {
		t.Error("JSON serialization should contain plaintext string")
	}
}

func TestEncryptionDeserializeLegacy(t *testing.T) {
	// Verifies that an un-obfuscated API key can be read from disk.

	cfg, err := Load("testdata/legacyapikey.xml", device1)
	if err != nil {
		t.Fatal(err)
	}

	const expected = "a non-obfuscated API key"
	if cfg.GUI().APIKey.String() != expected {
		t.Error("Expected to read API key from legacy config")
	}
}

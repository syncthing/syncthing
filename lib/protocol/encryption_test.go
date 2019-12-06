// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestEnDecryptName(t *testing.T) {
	var key [32]byte
	cases := []string{
		"",
		"foo",
		"a longer name/with/slashes not that they matter",
	}
	for _, tc := range cases {
		enc := encryptName(tc, &key)
		if tc != "" && strings.Contains(enc, tc) {
			t.Error("shouldn't contain plaintext")
		}
		dec, err := decryptName(enc, &key)
		if err != nil {
			t.Error(err)
		}
		if dec != tc {
			t.Error("mismatch after decryption")
		}
	}
}

func TestEnDecryptBytes(t *testing.T) {
	var key [32]byte
	cases := [][]byte{
		[]byte{},
		[]byte{1, 2, 3, 4, 5},
	}
	for _, tc := range cases {
		enc := encryptBytes(tc, &key)
		if len(tc) > 0 && bytes.Contains(enc, tc) {
			t.Error("shouldn't contain plaintext")
		}
		dec, err := decryptBytes(enc, &key)
		if err != nil {
			t.Error(err)
		}
		if !bytes.Equal(dec, tc) {
			t.Error("mismatch after decryption")
		}
	}
}

func TestEnDecryptFileInfo(t *testing.T) {
	var key [32]byte
	fi := FileInfo{
		Name:        "hello",
		Size:        45,
		Permissions: 0755,
		ModifiedS:   8080,
		Blocks: []BlockInfo{
			BlockInfo{
				Size: 45,
				Hash: []byte{1, 2, 3},
			},
		},
	}

	enc := encryptFileInfo(fi, &key)
	dec, err := decryptFileInfo(enc, &key)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(fi, dec) {
		t.Error("mismatch after decryption")
	}
}

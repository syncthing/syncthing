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
		"a longer name/with/slashes and spaces",
	}
	for _, tc := range cases {
		var prev string
		for i := 0; i < 5; i++ {
			enc := encryptName(tc, &key)
			if prev != "" && prev != enc {
				t.Error("name should always encrypt the same")
			}
			prev = enc
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
			t.Log(enc)
		}
	}
}

func TestEnDecryptBytes(t *testing.T) {
	var key [32]byte
	cases := [][]byte{
		{},
		{1, 2, 3, 4, 5},
	}
	for _, tc := range cases {
		var prev []byte
		for i := 0; i < 5; i++ {
			enc := encryptBytes(tc, &key)
			if bytes.Equal(enc, prev) {
				t.Error("encryption should not repeat")
			}
			prev = enc
			if len(tc) > 0 && bytes.Contains(enc, tc) {
				t.Error("shouldn't contain plaintext")
			}
			dec, err := DecryptBytes(enc, &key)
			if err != nil {
				t.Error(err)
			}
			if !bytes.Equal(dec, tc) {
				t.Error("mismatch after decryption")
			}
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
			{
				Size: 45,
				Hash: []byte{1, 2, 3},
			},
		},
	}

	enc := encryptFileInfo(fi, &key)
	dec, err := DecryptFileInfo(enc, &key)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(fi, dec) {
		t.Error("mismatch after decryption")
	}
}

func TestIsEncryptedParent(t *testing.T) {
	cases := []struct {
		path string
		is   bool
	}{
		{"", false},
		{".", false},
		{"/", false},
		{"12" + encryptedDirExtension, false},
		{"1" + encryptedDirExtension, true},
		{"1" + encryptedDirExtension + "/b", false},
		{"1" + encryptedDirExtension + "/bc", true},
		{"1" + encryptedDirExtension + "/bcd", false},
		{"1" + encryptedDirExtension + "/bc/foo", false},
		{"1.12/22", false},
	}
	for _, tc := range cases {
		if res := IsEncryptedParent(tc.path); res != tc.is {
			t.Errorf("%v: got %v, expected %v", tc.path, res, tc.is)
		}
	}
}

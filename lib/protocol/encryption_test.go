// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/syncthing/syncthing/lib/rand"
)

func TestEnDecryptName(t *testing.T) {
	pattern := regexp.MustCompile(
		fmt.Sprintf("^[0-9A-V]%s/[0-9A-V]{2}/([0-9A-V]{%d}/)*[0-9A-V]{1,%d}$",
			regexp.QuoteMeta(encryptedDirExtension),
			maxPathComponent, maxPathComponent-1))

	makeName := func(n int) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte('a' + i%26)
		}
		return string(b)
	}

	var key [32]byte
	cases := []string{
		"",
		"foo",
		"a longer name/with/slashes and spaces",
		makeName(maxPathComponent),
		makeName(1 + maxPathComponent),
		makeName(2 * maxPathComponent),
		makeName(1 + 2*maxPathComponent),
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
			if !pattern.MatchString(enc) {
				t.Fatalf("encrypted name %s doesn't match %s",
					enc, pattern)
			}

			dec, err := decryptName(enc, &key)
			if err != nil {
				t.Error(err)
			}
			if dec != tc {
				t.Error("mismatch after decryption")
			}
			t.Logf("%q encrypts as %q", tc, enc)
		}
	}
}

func TestDecryptNameInvalid(t *testing.T) {
	key := new([32]byte)
	for _, c := range []string{
		"T.syncthing-enc/OD",
		"T.syncthing-enc/OD/",
		"T.wrong-extension/OD/PHVDD67S7FI2K5QQMPSOFSK",
		"OD/PHVDD67S7FI2K5QQMPSOFSK",
	} {
		if _, err := decryptName(c, key); err == nil {
			t.Errorf("no error for %q", c)
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
				Offset: 0,
				Size:   45,
				Hash:   []byte{1, 2, 3},
			},
			{
				Offset: 45,
				Size:   45,
				Hash:   []byte{1, 2, 3},
			},
		},
	}

	enc := encryptFileInfo(fi, &key)
	if bytes.Equal(enc.Blocks[0].Hash, enc.Blocks[1].Hash) {
		t.Error("block hashes should not repeat when on different offsets")
	}
	again := encryptFileInfo(fi, &key)
	if !bytes.Equal(enc.Blocks[0].Hash, again.Blocks[0].Hash) {
		t.Error("block hashes should remain stable (0)")
	}
	if !bytes.Equal(enc.Blocks[1].Hash, again.Blocks[1].Hash) {
		t.Error("block hashes should remain stable (1)")
	}

	dec, err := DecryptFileInfo(enc, &key)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(fi, dec) {
		t.Error("mismatch after decryption")
	}
}

func TestIsEncryptedParent(t *testing.T) {
	comp := rand.String(maxPathComponent)
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
		{"1" + encryptedDirExtension + "/bc/" + comp, true},
		{"1" + encryptedDirExtension + "/bc/" + comp + "/" + comp, true},
		{"1" + encryptedDirExtension + "/bc/" + comp + "a", false},
		{"1" + encryptedDirExtension + "/bc/" + comp + "/a/" + comp, false},
	}
	for _, tc := range cases {
		if res := IsEncryptedParent(tc.path); res != tc.is {
			t.Errorf("%v: got %v, expected %v", tc.path, res, tc.is)
		}
	}
}

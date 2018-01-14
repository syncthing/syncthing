// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func newNoopNonceManager() nonceManager {
	return &noopNonceManager{
		nonces: make(map[string][]byte),
	}
}

type noopNonceManager struct {
	nonces map[string][]byte
}

func (m *noopNonceManager) getNameNonces(name string) []byte {
	if n, ok := m.nonces[name]; ok {
		return n
	}
	return make([]byte, aesBlockSize)
}

func (m *noopNonceManager) setNameNonce(name string, nonce []byte) {
	m.nonces[name] = nonce
}

func (noopNonceManager) getContentNonceStorage(string) *nonceStorage { return nil }
func (noopNonceManager) discardContentNonces(string)                 {}
func (noopNonceManager) populate() error                             { return nil }
func (noopNonceManager) flush()                                      {}

func TestEncryptedNameConversion(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	fs, err := newEncryptedFilesystem(`basic://Ag/` + dir)
	if err != nil {
		t.Fatal(err)
	}
	if !fs.encNames {
		t.Fatal("bad key")
	}
	fs.nonces = newNoopNonceManager()

	tests := []struct {
		plain     string
		encrypted string
	}{
		{"foo", "AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM"},
		{"foo.txt", "AAAAAAAAAAAAAAAAAAAAAAftPkZBG8MBp3snTHDtuDc"},
		{"foo/bar/baz", "AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAPjI2U4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAPjK2U4broFo38jSHTpvDM"},
		{".stfolder", ".stfolder"},
		{".stignore", ".stignore"},
		{"foo/.stfolder", "AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/.stfolder"},
		{"foo/.stignore", "AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/.stignore"},
		{"../../symlink", "../../AAAAAAAAAAAAAAAAAAAAABL7PARcDdwBp3snTHDtuDc"},
		{"..", ".."},
		{".", "."},
		{"././../../symlink", "././../../AAAAAAAAAAAAAAAAAAAAABL7PARcDdwBp3snTHDtuDc"},
		{"foo/bar/~syncthing~myfile.txt.tmp", "AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAPjI2U4broFo38jSHTpvDM/~syncthing~AAAAAAAAAAAAAAAAAAAAAAz7NwFZBpl81gYoQ3_itzg.tmp"},
		{"foo/bar/~syncthing~myfile.tmp", "AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAPjI2U4broFo38jSHTpvDM/~syncthing~AAAAAAAAAAAAAAAAAAAAAAz7NwFZBr0CpHgkT3PuuzQ.tmp"},
		{"foo/bar/.syncthing.myfile.txt.tmp", "AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAPjI2U4broFo38jSHTpvDM/.syncthing.AAAAAAAAAAAAAAAAAAAAAAz7NwFZBpl81gYoQ3_itzg.tmp"},
		{"foo/bar/.syncthing.myfile.tmp", "AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAPjI2U4broFo38jSHTpvDM/.syncthing.AAAAAAAAAAAAAAAAAAAAAAz7NwFZBr0CpHgkT3PuuzQ.tmp"},
		{"foo/bar/myfile~20060102-150405.txt", "AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAPjI2U4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAz7NwFZBpl81gYoQ3_itzg~20060102-150405"},
		{"foo/bar/myfile~20060102-150405", "AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAPjI2U4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAz7NwFZBr0CpHgkT3PuuzQ~20060102-150405"},
		{"foo/bar/myfile.sync-conflict-20060102-150405-7777777.txt", "AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAPjI2U4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAz7NwFZBpl81gYoQ3_itzg.sync-conflict-20060102-150405-7777777"},
		{"foo/bar/myfile.sync-conflict-20060102-150405-7777777", "AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAPjI2U4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAz7NwFZBr0CpHgkT3PuuzQ.sync-conflict-20060102-150405-7777777"},
		{"foo/bar/.syncthing.myfile.sync-conflict-20060102-150405-7777777.txt.tmp", "AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAPjI2U4broFo38jSHTpvDM/.syncthing.AAAAAAAAAAAAAAAAAAAAAAz7NwFZBpl81gYoQ3_itzg.sync-conflict-20060102-150405-7777777.tmp"},
		{"foo/bar/.syncthing.myfile.sync-conflict-20060102-150405-7777777.tmp", "AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAPjI2U4broFo38jSHTpvDM/.syncthing.AAAAAAAAAAAAAAAAAAAAAAz7NwFZBr0CpHgkT3PuuzQ.sync-conflict-20060102-150405-7777777.tmp"},
		{"foo/bar/.syncthing.myfile.sync-conflict-20060102-150405-7777777.sync-conflict-20060102-999999-7777777.txt.tmp", "AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAPjI2U4broFo38jSHTpvDM/.syncthing.AAAAAAAAAAAAAAAAAAAAAAz7NwFZBpl81gYoQ3_itzg.sync-conflict-20060102-150405-7777777.sync-conflict-20060102-999999-7777777.tmp"},
		{"foo/bar/.syncthing.myfile.sync-conflict-20060102-150405-7777777.sync-conflict-20060102-999999-7777777~20060102-150405.txt.tmp", "AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAPjI2U4broFo38jSHTpvDM/.syncthing.AAAAAAAAAAAAAAAAAAAAAAz7NwFZBpl81gYoQ3_itzg.sync-conflict-20060102-150405-7777777.sync-conflict-20060102-999999-7777777~20060102-150405.tmp"},
	}

	if runtime.GOOS == "windows" {
		for i, test := range tests {
			test.plain = filepath.FromSlash(test.plain)
			test.encrypted = filepath.FromSlash(test.encrypted)
			tests[i] = test
		}
	}

	for i, test := range tests {
		encName, err := fs.encryptName(test.plain)
		if err != nil {
			t.Errorf("%d encryption failed: %s", i, err)
			continue
		}
		if encName != test.encrypted {
			t.Errorf("%d enc check failed: %s != %s", i, encName, test.encrypted)
			continue
		}
		decrName, err := fs.decryptName(encName)
		if err != nil {
			t.Errorf("%d decryption failed: %s", i, err)
			continue
		}
		if decrName != test.plain {
			t.Errorf("%d plain check failed: %s != %s", i, decrName, test.plain)
		}
	}
}

func TestEncryptedGlob(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	fs, err := newEncryptedFilesystem(`basic://Ag/` + dir)
	if err != nil {
		t.Fatal(err)
	}
	if !fs.encNames {
		t.Fatal("bad key")
	}
	fs.nonces = newNoopNonceManager()

	subdir := filepath.Join(dir, "foo")
	if err := os.MkdirAll(subdir, 0777); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{
		"myfile~20001122-334401",
		"myfile~20001122-334402",
		"myfile~20001122-334401.txt",
		"myfile~20001122-334402.txt",
		"myfile.sync-conflict-20001122-334401-7777777",
		"myfile.sync-conflict-20001122-334402-7777777",
		"myfile.sync-conflict-20001122-334401-7777777.txt",
		"myfile.sync-conflict-20001122-334402-7777777.txt",
	} {
		fd, err := os.Create(filepath.Join(subdir, name))
		if err != nil {
			t.Fatal(err)
		}
		fd.Close()
	}

	for _, test := range []struct {
		pattern  string
		expected []string
	}{
		{
			"AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAz7NwFZBr0CpHgkT3PuuzQ~[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-[0-9][0-9][0-9][0-9][0-9][0-9]",
			[]string{
				"AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAz7NwFZBr0CpHgkT3PuuzQ~20001122-334401",
				"AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAz7NwFZBr0CpHgkT3PuuzQ~20001122-334402",
			},
		}, {
			"AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAz7NwFZBpl81gYoQ3_itzg~[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-[0-9][0-9][0-9][0-9][0-9][0-9]",
			[]string{
				"AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAz7NwFZBpl81gYoQ3_itzg~20001122-334401",
				"AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAz7NwFZBpl81gYoQ3_itzg~20001122-334402",
			},
		}, {
			"AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAz7NwFZBr0CpHgkT3PuuzQ.sync-conflict-????????-??????-???????",
			[]string{
				"AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAz7NwFZBr0CpHgkT3PuuzQ.sync-conflict-20001122-334401-7777777",
				"AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAz7NwFZBr0CpHgkT3PuuzQ.sync-conflict-20001122-334402-7777777",
			},
		}, {
			"AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAz7NwFZBpl81gYoQ3_itzg.sync-conflict-????????-??????-???????",
			[]string{
				"AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAz7NwFZBpl81gYoQ3_itzg.sync-conflict-20001122-334401-7777777",
				"AAAAAAAAAAAAAAAAAAAAAAftPmU4broFo38jSHTpvDM/AAAAAAAAAAAAAAAAAAAAAAz7NwFZBpl81gYoQ3_itzg.sync-conflict-20001122-334402-7777777",
			},
		},
	} {
		test.pattern = filepath.FromSlash(test.pattern)
		for i := range test.expected {
			test.expected[i] = filepath.FromSlash(test.expected[i])
		}

		names, err := fs.Glob(test.pattern)
		if err != nil {
			t.Errorf("%s: %s", test.pattern, err)
			continue
		}
		if len(names) != len(test.expected) {
			t.Errorf("length mismatch: %d != %d", len(names), len(test.expected))
			continue
		}

		for i := range names {
			if names[i] != test.expected[i] {
				t.Errorf("%s: (%d) %s != %s", test.pattern, i, names[i], test.expected[i])
			}
		}
	}
}

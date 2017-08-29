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

type noopNonceManager struct {
	nonceManager
}

func (noopNonceManager) flush() {}

func TestEncryptedNameConversion(t *testing.T) {
	fs, err := newEncryptedFilesystem(`basic://Ag/.`)
	if err != nil {
		t.Fatal(err)
	}
	if !fs.encNames {
		t.Fatal("bad key")
	}
	fs.nonces = noopNonceManager{fs.nonces}

	tests := []struct {
		plain     string
		encrypted string
	}{
		{"foo", "KRA0WpnHjj3Xnu8c4XxeudOcfH5hTH5xlNzf-n1qlW0"},
		{"foo.txt", "qrf-FUij196Qh3_8UZ7_qD0T1P1F70SQfd_CwdQmHpE"},
		{"foo/bar/baz", "KRA0WpnHjj3Xnu8c4XxeudOcfH5hTH5xlNzf-n1qlW0/MPMLGdyDagNaFL6QWRFo0QkBhhB3YyUouzLLbndpOGo/D7YVqeu7IU5UxmJHElca30hW3f5VIZ4gitvRwJhL9no"},
		{".stfolder", ".stfolder"},
		{".stignore", ".stignore"},
		{"foo/.stfolder", "KRA0WpnHjj3Xnu8c4XxeudOcfH5hTH5xlNzf-n1qlW0/.stfolder"},
		{"foo/.stignore", "KRA0WpnHjj3Xnu8c4XxeudOcfH5hTH5xlNzf-n1qlW0/.stignore"},
		{"../../symlink", "../../L4vV2dgapACgK8s0Rz4omAmSIMU529zsIsLFFxR30Nw"},
		{"..", ".."},
		{".", "."},
		{"././../../symlink", "././../../L4vV2dgapACgK8s0Rz4omAmSIMU529zsIsLFFxR30Nw"},
		{"foo/bar/~syncthing~myfile.txt.tmp", "KRA0WpnHjj3Xnu8c4XxeudOcfH5hTH5xlNzf-n1qlW0/MPMLGdyDagNaFL6QWRFo0QkBhhB3YyUouzLLbndpOGo/~syncthing~wwWbSrrvc_2zzH-pb7Fle8gBHC0IR6yBCOS-bYcfqZA.tmp"},
		{"foo/bar/~syncthing~myfile.tmp", "KRA0WpnHjj3Xnu8c4XxeudOcfH5hTH5xlNzf-n1qlW0/MPMLGdyDagNaFL6QWRFo0QkBhhB3YyUouzLLbndpOGo/~syncthing~d4EfIoYmMxH6upQtuzXDvKfPfW-NSF-cVf3eYkN3cgU.tmp"},
		{"foo/bar/.syncthing.myfile.txt.tmp", "KRA0WpnHjj3Xnu8c4XxeudOcfH5hTH5xlNzf-n1qlW0/MPMLGdyDagNaFL6QWRFo0QkBhhB3YyUouzLLbndpOGo/.syncthing.wwWbSrrvc_2zzH-pb7Fle8gBHC0IR6yBCOS-bYcfqZA.tmp"},
		{"foo/bar/.syncthing.myfile.tmp", "KRA0WpnHjj3Xnu8c4XxeudOcfH5hTH5xlNzf-n1qlW0/MPMLGdyDagNaFL6QWRFo0QkBhhB3YyUouzLLbndpOGo/.syncthing.d4EfIoYmMxH6upQtuzXDvKfPfW-NSF-cVf3eYkN3cgU.tmp"},
		{"foo/bar/myfile~20060102-150405.txt", "KRA0WpnHjj3Xnu8c4XxeudOcfH5hTH5xlNzf-n1qlW0/MPMLGdyDagNaFL6QWRFo0QkBhhB3YyUouzLLbndpOGo/wwWbSrrvc_2zzH-pb7Fle8gBHC0IR6yBCOS-bYcfqZA~20060102-150405"},
		{"foo/bar/myfile~20060102-150405", "KRA0WpnHjj3Xnu8c4XxeudOcfH5hTH5xlNzf-n1qlW0/MPMLGdyDagNaFL6QWRFo0QkBhhB3YyUouzLLbndpOGo/d4EfIoYmMxH6upQtuzXDvKfPfW-NSF-cVf3eYkN3cgU~20060102-150405"},
		{"foo/bar/myfile.sync-conflict-20060102-150405-7777777.txt", "KRA0WpnHjj3Xnu8c4XxeudOcfH5hTH5xlNzf-n1qlW0/MPMLGdyDagNaFL6QWRFo0QkBhhB3YyUouzLLbndpOGo/wwWbSrrvc_2zzH-pb7Fle8gBHC0IR6yBCOS-bYcfqZA.sync-conflict-20060102-150405-7777777"},
		{"foo/bar/myfile.sync-conflict-20060102-150405-7777777", "KRA0WpnHjj3Xnu8c4XxeudOcfH5hTH5xlNzf-n1qlW0/MPMLGdyDagNaFL6QWRFo0QkBhhB3YyUouzLLbndpOGo/d4EfIoYmMxH6upQtuzXDvKfPfW-NSF-cVf3eYkN3cgU.sync-conflict-20060102-150405-7777777"},
		{"foo/bar/.syncthing.myfile.sync-conflict-20060102-150405-7777777.txt.tmp", "KRA0WpnHjj3Xnu8c4XxeudOcfH5hTH5xlNzf-n1qlW0/MPMLGdyDagNaFL6QWRFo0QkBhhB3YyUouzLLbndpOGo/.syncthing.wwWbSrrvc_2zzH-pb7Fle8gBHC0IR6yBCOS-bYcfqZA.sync-conflict-20060102-150405-7777777.tmp"},
		{"foo/bar/.syncthing.myfile.sync-conflict-20060102-150405-7777777.tmp", "KRA0WpnHjj3Xnu8c4XxeudOcfH5hTH5xlNzf-n1qlW0/MPMLGdyDagNaFL6QWRFo0QkBhhB3YyUouzLLbndpOGo/.syncthing.d4EfIoYmMxH6upQtuzXDvKfPfW-NSF-cVf3eYkN3cgU.sync-conflict-20060102-150405-7777777.tmp"},
		{"foo/bar/.syncthing.myfile.sync-conflict-20060102-150405-7777777.sync-conflict-20060102-999999-7777777.txt.tmp", "KRA0WpnHjj3Xnu8c4XxeudOcfH5hTH5xlNzf-n1qlW0/MPMLGdyDagNaFL6QWRFo0QkBhhB3YyUouzLLbndpOGo/.syncthing.wwWbSrrvc_2zzH-pb7Fle8gBHC0IR6yBCOS-bYcfqZA.sync-conflict-20060102-150405-7777777.sync-conflict-20060102-999999-7777777.tmp"},
		{"foo/bar/.syncthing.myfile.sync-conflict-20060102-150405-7777777.sync-conflict-20060102-999999-7777777~20060102-150405.txt.tmp", "KRA0WpnHjj3Xnu8c4XxeudOcfH5hTH5xlNzf-n1qlW0/MPMLGdyDagNaFL6QWRFo0QkBhhB3YyUouzLLbndpOGo/.syncthing.wwWbSrrvc_2zzH-pb7Fle8gBHC0IR6yBCOS-bYcfqZA.sync-conflict-20060102-150405-7777777.sync-conflict-20060102-999999-7777777~20060102-150405.tmp"},
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
	fs.nonces = noopNonceManager{fs.nonces}

	subdir := filepath.Join(dir, "foo")
	if err := os.MkdirAll(subdir, 0777); err != nil {
		t.Fatal(err)
	}

	fs.nonces.setNameNonce("foo", make([]byte, aesBlockSize))
	fs.nonces.setNameNonce("myfile", make([]byte, aesBlockSize))
	fs.nonces.setNameNonce("myfile.txt", make([]byte, aesBlockSize))
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

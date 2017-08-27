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

func TestEncryptedNameConversion(t *testing.T) {
	fs, err := newEncryptedFilesystem(`basic://Ag/.`)
	if err != nil {
		t.Error(err)
	}
	if !fs.encNames {
		t.Fatal("bad key")
	}

	tests := []struct {
		plain     string
		encrypted string
	}{

		{"foo", "Td3oPdBHOTz2UY2kZMA8uQ"},
		{"foo.txt", "TekIh0KLDmM0LwwDrnbuIg"},
		{"foo/bar/baz", "Td3oPdBHOTz2UY2kZMA8uQ/2KBOw_LKTp9-_o4GXQgHtA/B2AcsD0DO9A-oS02lpkQ-A"},
		{".stfolder", ".stfolder"},
		{".stignore", ".stignore"},
		{"foo/.stfolder", "Td3oPdBHOTz2UY2kZMA8uQ/.stfolder"},
		{"foo/.stignore", "Td3oPdBHOTz2UY2kZMA8uQ/.stignore"},
		{"../../symlink", "../../PHTC7GrH_hpNRlmGV8UytA"},
		{"..", ".."},
		{".", "."},
		{"././../../symlink", "././../../PHTC7GrH_hpNRlmGV8UytA"},
		{"foo/bar/~syncthing~myfile.txt.tmp", "Td3oPdBHOTz2UY2kZMA8uQ/2KBOw_LKTp9-_o4GXQgHtA/~syncthing~2kAd30NvqbrDTvnlBnQZfA.tmp"},
		{"foo/bar/~syncthing~myfile.tmp", "Td3oPdBHOTz2UY2kZMA8uQ/2KBOw_LKTp9-_o4GXQgHtA/~syncthing~4NLxXqSM-ic4dl31psNS-Q.tmp"},
		{"foo/bar/.syncthing.myfile.txt.tmp", "Td3oPdBHOTz2UY2kZMA8uQ/2KBOw_LKTp9-_o4GXQgHtA/.syncthing.2kAd30NvqbrDTvnlBnQZfA.tmp"},
		{"foo/bar/.syncthing.myfile.tmp", "Td3oPdBHOTz2UY2kZMA8uQ/2KBOw_LKTp9-_o4GXQgHtA/.syncthing.4NLxXqSM-ic4dl31psNS-Q.tmp"},
		{"foo/bar/myfile~20060102-150405.txt", "Td3oPdBHOTz2UY2kZMA8uQ/2KBOw_LKTp9-_o4GXQgHtA/2kAd30NvqbrDTvnlBnQZfA~20060102-150405"},
		{"foo/bar/myfile~20060102-150405", "Td3oPdBHOTz2UY2kZMA8uQ/2KBOw_LKTp9-_o4GXQgHtA/4NLxXqSM-ic4dl31psNS-Q~20060102-150405"},
		{"foo/bar/myfile.sync-conflict-20060102-150405-7777777.txt", "Td3oPdBHOTz2UY2kZMA8uQ/2KBOw_LKTp9-_o4GXQgHtA/2kAd30NvqbrDTvnlBnQZfA.sync-conflict-20060102-150405-7777777"},
		{"foo/bar/myfile.sync-conflict-20060102-150405-7777777", "Td3oPdBHOTz2UY2kZMA8uQ/2KBOw_LKTp9-_o4GXQgHtA/4NLxXqSM-ic4dl31psNS-Q.sync-conflict-20060102-150405-7777777"},
		{"foo/bar/.syncthing.myfile.sync-conflict-20060102-150405-7777777.txt.tmp", "Td3oPdBHOTz2UY2kZMA8uQ/2KBOw_LKTp9-_o4GXQgHtA/.syncthing.2kAd30NvqbrDTvnlBnQZfA.sync-conflict-20060102-150405-7777777.tmp"},
		{"foo/bar/.syncthing.myfile.sync-conflict-20060102-150405-7777777.tmp", "Td3oPdBHOTz2UY2kZMA8uQ/2KBOw_LKTp9-_o4GXQgHtA/.syncthing.4NLxXqSM-ic4dl31psNS-Q.sync-conflict-20060102-150405-7777777.tmp"},
		{"foo/bar/.syncthing.myfile.sync-conflict-20060102-150405-7777777.sync-conflict-20060102-999999-7777777.txt.tmp", "Td3oPdBHOTz2UY2kZMA8uQ/2KBOw_LKTp9-_o4GXQgHtA/.syncthing.2kAd30NvqbrDTvnlBnQZfA.sync-conflict-20060102-150405-7777777.sync-conflict-20060102-999999-7777777.tmp"},
		{"foo/bar/.syncthing.myfile.sync-conflict-20060102-150405-7777777.sync-conflict-20060102-999999-7777777~20060102-150405.txt.tmp", "Td3oPdBHOTz2UY2kZMA8uQ/2KBOw_LKTp9-_o4GXQgHtA/.syncthing.2kAd30NvqbrDTvnlBnQZfA.sync-conflict-20060102-150405-7777777.sync-conflict-20060102-999999-7777777~20060102-150405.tmp"},
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
		t.Error(err)
	}
	if !fs.encNames {
		t.Fatal("bad key")
	}

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
			"Td3oPdBHOTz2UY2kZMA8uQ/4NLxXqSM-ic4dl31psNS-Q~[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-[0-9][0-9][0-9][0-9][0-9][0-9]",
			[]string{
				"Td3oPdBHOTz2UY2kZMA8uQ/4NLxXqSM-ic4dl31psNS-Q~20001122-334401",
				"Td3oPdBHOTz2UY2kZMA8uQ/4NLxXqSM-ic4dl31psNS-Q~20001122-334402",
			},
		}, {
			"Td3oPdBHOTz2UY2kZMA8uQ/2kAd30NvqbrDTvnlBnQZfA~[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-[0-9][0-9][0-9][0-9][0-9][0-9]",
			[]string{
				"Td3oPdBHOTz2UY2kZMA8uQ/2kAd30NvqbrDTvnlBnQZfA~20001122-334401",
				"Td3oPdBHOTz2UY2kZMA8uQ/2kAd30NvqbrDTvnlBnQZfA~20001122-334402",
			},
		}, {
			"Td3oPdBHOTz2UY2kZMA8uQ/4NLxXqSM-ic4dl31psNS-Q.sync-conflict-????????-??????-???????",
			[]string{
				"Td3oPdBHOTz2UY2kZMA8uQ/4NLxXqSM-ic4dl31psNS-Q.sync-conflict-20001122-334401-7777777",
				"Td3oPdBHOTz2UY2kZMA8uQ/4NLxXqSM-ic4dl31psNS-Q.sync-conflict-20001122-334402-7777777",
			},
		}, {
			"Td3oPdBHOTz2UY2kZMA8uQ/2kAd30NvqbrDTvnlBnQZfA.sync-conflict-????????-??????-???????",
			[]string{
				"Td3oPdBHOTz2UY2kZMA8uQ/2kAd30NvqbrDTvnlBnQZfA.sync-conflict-20001122-334401-7777777",
				"Td3oPdBHOTz2UY2kZMA8uQ/2kAd30NvqbrDTvnlBnQZfA.sync-conflict-20001122-334402-7777777",
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

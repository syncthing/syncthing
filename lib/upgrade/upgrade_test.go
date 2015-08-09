// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build !noupgrade

package upgrade

import (
	"encoding/json"
	"os"
	"testing"
)

var versions = []struct {
	a, b string
	r    Relation
}{
	{"0.1.2", "0.1.2", Equal},
	{"0.1.3", "0.1.2", Newer},
	{"0.1.1", "0.1.2", Older},
	{"0.3.0", "0.1.2", MajorNewer},
	{"0.0.9", "0.1.2", MajorOlder},
	{"1.3.0", "1.1.2", Newer},
	{"1.0.9", "1.1.2", Older},
	{"2.3.0", "1.1.2", MajorNewer},
	{"1.0.9", "2.1.2", MajorOlder},
	{"1.1.2", "0.1.2", MajorNewer},
	{"0.1.2", "1.1.2", MajorOlder},
	{"0.1.10", "0.1.9", Newer},
	{"0.10.0", "0.2.0", MajorNewer},
	{"30.10.0", "4.9.0", MajorNewer},
	{"0.9.0-beta7", "0.9.0-beta6", Newer},
	{"0.9.0-beta7", "1.0.0-alpha", MajorOlder},
	{"1.0.0-alpha", "1.0.0-alpha.1", Older},
	{"1.0.0-alpha.1", "1.0.0-alpha.beta", Older},
	{"1.0.0-alpha.beta", "1.0.0-beta", Older},
	{"1.0.0-beta", "1.0.0-beta.2", Older},
	{"1.0.0-beta.2", "1.0.0-beta.11", Older},
	{"1.0.0-beta.11", "1.0.0-rc.1", Older},
	{"1.0.0-rc.1", "1.0.0", Older},
	{"1.0.0+45", "1.0.0+23-dev-foo", Equal},
	{"1.0.0-beta.23+45", "1.0.0-beta.23+23-dev-foo", Equal},
	{"1.0.0-beta.3+99", "1.0.0-beta.24+0", Older},

	{"v1.1.2", "1.1.2", Equal},
	{"v1.1.2", "V1.1.2", Equal},
	{"1.1.2", "V1.1.2", Equal},
}

func TestCompareVersions(t *testing.T) {
	for _, v := range versions {
		if r := CompareVersions(v.a, v.b); r != v.r {
			t.Errorf("compareVersions(%q, %q): %d != %d", v.a, v.b, r, v.r)
		}
	}
}

var upgrades = map[string]string{
	"v0.10.21":                        "v0.10.30",
	"v0.10.29":                        "v0.10.30",
	"v0.10.31":                        "v0.10.30",
	"v0.10.0-alpha":                   "v0.10.30",
	"v0.10.0-beta":                    "v0.11.0-beta0",
	"v0.11.0-beta0+40-g53cb66e-dirty": "v0.11.0-beta0",
}

func TestGithubRelease(t *testing.T) {
	fd, err := os.Open("testdata/github-releases.json")
	if err != nil {
		t.Errorf("Missing github-release test data")
	}
	defer fd.Close()

	var rels []Release
	json.NewDecoder(fd).Decode(&rels)

	for old, target := range upgrades {
		upgrade, err := SelectLatestRelease(old, rels)
		if err != nil {
			t.Error("Error retrieving latest version", err)
		}
		if upgrade.Tag != target {
			t.Errorf("Invalid upgrade release: %v -> %v, but got %v", old, target, upgrade.Tag)
		}
	}
}

func TestErrorRelease(t *testing.T) {
	_, err := SelectLatestRelease("v0.11.0-beta", nil)
	if err == nil {
		t.Error("Should return an error when no release were available")
	}
}

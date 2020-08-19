// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"fmt"
	"io/ioutil"
	"testing"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		longVersion string
		parsed      version
	}{
		{
			longVersion: `syncthing v1.1.4-rc.1+30-g6aaae618-dirty-crashrep "Erbium Earthworm" (go1.12.5 darwin-amd64) jb@kvin.kastelo.net 2019-05-23 16:08:14 UTC`,
			parsed: version{
				version:  "v1.1.4-rc.1+30-g6aaae618-dirty-crashrep",
				tag:      "v1.1.4-rc.1",
				commit:   "6aaae618",
				codename: "Erbium Earthworm",
				runtime:  "go1.12.5",
				goos:     "darwin",
				goarch:   "amd64",
				builder:  "jb@kvin.kastelo.net",
			},
		},
		{
			longVersion: `syncthing v1.1.4-rc.1+30-g6aaae618-dirty-crashrep "Erbium Earthworm" (go1.12.5 darwin-amd64) jb@kvin.kastelo.net 2019-05-23 16:08:14 UTC [foo, bar]`,
			parsed: version{
				version:  "v1.1.4-rc.1+30-g6aaae618-dirty-crashrep",
				tag:      "v1.1.4-rc.1",
				commit:   "6aaae618",
				codename: "Erbium Earthworm",
				runtime:  "go1.12.5",
				goos:     "darwin",
				goarch:   "amd64",
				builder:  "jb@kvin.kastelo.net",
				extra:    []string{"foo", "bar"},
			},
		},
	}

	for _, tc := range cases {
		v, err := parseVersion(tc.longVersion)
		if err != nil {
			t.Errorf("%s\nerror: %v\n", tc.longVersion, err)
			continue
		}
		if fmt.Sprint(v) != fmt.Sprint(tc.parsed) {
			t.Errorf("%s\nA: %v\nE: %v\n", tc.longVersion, v, tc.parsed)
		}
	}
}

func TestParseReport(t *testing.T) {
	bs, err := ioutil.ReadFile("_testdata/panic.log")
	if err != nil {
		t.Fatal(err)
	}

	pkt, err := parseReport("1/2/345", bs)
	if err != nil {
		t.Fatal(err)
	}

	bs, err = pkt.JSON()
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("%s\n", bs)
}

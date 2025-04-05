// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package build

import (
	"fmt"
	"testing"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		longVersion string
		parsed      VersionParts
	}{
		{
			longVersion: `syncthing v1.1.4-rc.1+30-g6aaae618-dirty-crashrep "Erbium Earthworm" (go1.12.5 darwin-amd64) jb@kvin.kastelo.net 2019-05-23 16:08:14 UTC`,
			parsed: VersionParts{
				Version:  "v1.1.4-rc.1+30-g6aaae618-dirty-crashrep",
				Tag:      "v1.1.4-rc.1",
				Commit:   "6aaae618",
				Codename: "Erbium Earthworm",
				Runtime:  "go1.12.5",
				GOOS:     "darwin",
				GOARCH:   "amd64",
				Builder:  "jb@kvin.kastelo.net",
			},
		},
		{
			longVersion: `syncthing v1.1.4-rc.1+30-g6aaae618-dirty-crashrep "Erbium Earthworm" (go1.12.5 darwin-amd64) jb@kvin.kastelo.net 2019-05-23 16:08:14 UTC [foo, bar]`,
			parsed: VersionParts{
				Version:  "v1.1.4-rc.1+30-g6aaae618-dirty-crashrep",
				Tag:      "v1.1.4-rc.1",
				Commit:   "6aaae618",
				Codename: "Erbium Earthworm",
				Runtime:  "go1.12.5",
				GOOS:     "darwin",
				GOARCH:   "amd64",
				Builder:  "jb@kvin.kastelo.net",
				Extra:    []string{"foo", "bar"},
			},
		},
		{
			longVersion: `syncthing v1.23.7-dev.26.gdf7b56ae-stversionextra "Fermium Flea" (go1.20.5 darwin-arm64) jb@ok.kastelo.net 2023-07-12 06:55:26 UTC [Some Wrapper, purego, stnoupgrade]`,
			parsed: VersionParts{
				Version:  "v1.23.7-dev.26.gdf7b56ae-stversionextra",
				Tag:      "v1.23.7-dev",
				Commit:   "df7b56ae",
				Codename: "Fermium Flea",
				Runtime:  "go1.20.5",
				GOOS:     "darwin",
				GOARCH:   "arm64",
				Builder:  "jb@ok.kastelo.net",
				Extra:    []string{"Some Wrapper", "purego", "stnoupgrade"},
			},
		},
	}

	for _, tc := range cases {
		v, err := ParseVersion(tc.longVersion)
		if err != nil {
			t.Errorf("%s\nerror: %v\n", tc.longVersion, err)
			continue
		}
		if fmt.Sprint(v) != fmt.Sprint(tc.parsed) {
			t.Errorf("%s\nA: %v\nE: %v\n", tc.longVersion, v, tc.parsed)
		}
	}
}

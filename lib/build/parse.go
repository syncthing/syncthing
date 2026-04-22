// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package build

import (
	"errors"
	"regexp"
	"strings"
)

// syncthing v1.1.4-rc.1+30-g6aaae618-dirty-crashrep "Erbium Earthworm" (go1.12.5 darwin-amd64) jb@kvin.kastelo.net 2019-05-23 16:08:14 UTC [foo, bar]
// or, somewhere along the way the "+" in the version tag disappeared:
// syncthing v1.23.7-dev.26.gdf7b56ae.dirty-stversionextra "Fermium Flea" (go1.20.5 darwin-arm64) jb@ok.kastelo.net 2023-07-12 06:55:26 UTC [Some Wrapper, purego, stnoupgrade]
var (
	longVersionRE = regexp.MustCompile(`syncthing\s+(v[^\s]+)\s+"([^"]+)"\s\(([^\s]+)\s+([^-]+)-([^)]+)\)\s+([^\s]+)[^\[]*(?:\[(.+)\])?$`)
	gitExtraRE    = regexp.MustCompile(`\.\d+\.g[0-9a-f]+`) // ".1.g6aaae618"
	gitExtraSepRE = regexp.MustCompile(`[.-]`)              // dot or dash
)

type VersionParts struct {
	Version  string   // "v1.1.4-rc.1+30-g6aaae618-dirty-crashrep"
	Tag      string   // "v1.1.4-rc.1"
	Commit   string   // "6aaae618", blank when absent
	Codename string   // "Erbium Earthworm"
	Runtime  string   // "go1.12.5"
	GOOS     string   // "darwin"
	GOARCH   string   // "amd64"
	Builder  string   // "jb@kvin.kastelo.net"
	Extra    []string // "foo", "bar"
}

func (v VersionParts) Environment() string {
	if v.Commit != "" {
		return "Development"
	}
	if strings.Contains(v.Tag, "-rc.") {
		return "Candidate"
	}
	if strings.Contains(v.Tag, "-") {
		return "Beta"
	}
	return "Stable"
}

func ParseVersion(line string) (VersionParts, error) {
	m := longVersionRE.FindStringSubmatch(line)
	if len(m) == 0 {
		return VersionParts{}, errors.New("unintelligible version string")
	}

	v := VersionParts{
		Version:  m[1],
		Codename: m[2],
		Runtime:  m[3],
		GOOS:     m[4],
		GOARCH:   m[5],
		Builder:  m[6],
	}

	// Split the version tag into tag and commit. This is old style
	// v1.2.3-something.4+11-g12345678 or newer with just dots
	// v1.2.3-something.4.11.g12345678 or v1.2.3-dev.11.g12345678.
	parts := []string{v.Version}
	if strings.Contains(v.Version, "+") {
		parts = strings.Split(v.Version, "+")
	} else {
		idxs := gitExtraRE.FindStringIndex(v.Version)
		if len(idxs) > 0 {
			parts = []string{v.Version[:idxs[0]], v.Version[idxs[0]+1:]}
		}
	}
	v.Tag = parts[0]
	if len(parts) > 1 {
		fields := gitExtraSepRE.Split(parts[1], -1)
		if len(fields) >= 2 && strings.HasPrefix(fields[1], "g") {
			v.Commit = fields[1][1:]
		}
	}

	if len(m) >= 8 && m[7] != "" {
		tags := strings.Split(m[7], ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
		v.Extra = tags
	}

	return v, nil
}

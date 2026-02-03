// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package build

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// syncthing v1.1.4-rc.1+30-g6aaae618-dirty-crashrep "Erbium Earthworm" (go1.12.5 darwin-amd64) jb@kvin.kastelo.net 2019-05-23 16:08:14 UTC [foo, bar]
// or, somewhere along the way the "+" in the version tag disappeared:
// syncthing v1.23.7-dev.26.gdf7b56ae.dirty-stversionextra "Fermium Flea" (go1.20.5 darwin-arm64) jb@ok.kastelo.net 2023-07-12 06:55:26 UTC [Some Wrapper, purego, stnoupgrade]
// or, the new structured log format:
// 2026-02-03 08:49:09 INF Starting Syncthing (version=v2.0.14-rc.2.dev.2.gb40f2acd.dirty-startuplogs codename="Hafnium Hornet" build.user=jb ...)
var (
	longVersionRE = regexp.MustCompile(`syncthing\s+(v[^\s]+)\s+"([^"]+)"\s\(([^\s]+)\s+([^-]+)-([^)]+)\)\s+([^\s]+)[^\[]*(?:\[(.+)\])?$`)
	structuredRE  = regexp.MustCompile(`Starting Syncthing \((.+)\)`)
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
	if m := structuredRE.FindStringSubmatch(line); len(m) > 0 {
		return parseStructured(m[1])
	}

	if m := longVersionRE.FindStringSubmatch(line); len(m) > 0 {
		return parseClassic(m)
	}

	return VersionParts{}, errors.New("unintelligible version string")
}

func parseClassic(m []string) (VersionParts, error) {
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
	v.Tag, v.Commit = splitVersionTag(v.Version)

	if len(m) >= 8 && m[7] != "" {
		tags := strings.Split(m[7], ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
		v.Extra = tags
	}

	return v, nil
}

func parseStructured(attrs string) (VersionParts, error) {
	kvs := parseStructuredKVs(attrs)
	if kvs["version"] == "" {
		return VersionParts{}, errors.New("missing version in structured format")
	}

	vp := VersionParts{
		Version:  kvs["version"],
		Codename: kvs["codename"],
		Builder:  fmt.Sprintf("%s@%s", kvs["build.user"], kvs["build.host"]),
		Runtime:  kvs["runtime.version"],
		GOOS:     kvs["runtime.goos"],
		GOARCH:   kvs["runtime.goarch"],
	}

	vp.Tag, vp.Commit = splitVersionTag(vp.Version)

	if tags := kvs["tags"]; tags != "" {
		tags := strings.Split(kvs["tags"], ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
		vp.Extra = tags
	}

	return vp, nil
}

func parseStructuredKVs(attrs string) map[string]string {
	res := make(map[string]string)

	for len(attrs) > 0 {
		attrs = strings.TrimLeft(attrs, " ")
		if attrs == "" {
			break
		}

		// Find key
		eqIdx := strings.Index(attrs, "=")
		if eqIdx == -1 {
			break
		}
		key := attrs[:eqIdx]
		attrs = attrs[eqIdx+1:]

		var value string
		if len(attrs) > 0 && attrs[0] == '"' {
			// Quoted value - use strconv to properly handle escapes
			quoted, err := strconv.QuotedPrefix(attrs)
			if err != nil {
				break
			}
			value, err = strconv.Unquote(quoted)
			if err != nil {
				break
			}
			attrs = attrs[len(quoted):]
		} else {
			// Unquoted value - find next space
			spaceIdx := strings.Index(attrs, " ")
			if spaceIdx == -1 {
				value = attrs
				attrs = ""
			} else {
				value = attrs[:spaceIdx]
				attrs = attrs[spaceIdx:]
			}
		}

		res[key] = value
	}

	return res
}

func splitVersionTag(version string) (tag, commit string) {
	parts := []string{version}
	if strings.Contains(version, "+") {
		parts = strings.Split(version, "+")
	} else {
		idxs := gitExtraRE.FindStringIndex(version)
		if len(idxs) > 0 {
			parts = []string{version[:idxs[0]], version[idxs[0]+1:]}
		}
	}
	tag = parts[0]
	if len(parts) > 1 {
		fields := gitExtraSepRE.Split(parts[1], -1)
		if len(fields) >= 2 && strings.HasPrefix(fields[1], "g") {
			commit = fields[1][1:]
		}
	}
	return tag, commit
}

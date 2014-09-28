// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package upgrade downloads and compares releases, and upgrades the running binary.
package upgrade

import (
	"errors"
	"strconv"
	"strings"
)

type Release struct {
	Tag        string  `json:"tag_name"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

type Asset struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

var (
	ErrVersionUpToDate    = errors.New("current version is up to date")
	ErrVersionUnknown     = errors.New("couldn't fetch release information")
	ErrUpgradeUnsupported = errors.New("upgrade unsupported")
	ErrUpgradeInProgress  = errors.New("upgrade already in progress")
	upgradeUnlocked       = make(chan bool, 1)
)

func init() {
	upgradeUnlocked <- true
}

// A wrapper around actual implementations
func UpgradeTo(rel Release, archExtra string) error {
	select {
	case <-upgradeUnlocked:
		err := upgradeTo(rel, archExtra)
		// If we've failed to upgrade, unlock so that another attempt could be made
		if err != nil {
			upgradeUnlocked <- true
		}
		return err
	default:
		return ErrUpgradeInProgress
	}
}

// Returns 1 if a>b, -1 if a<b and 0 if they are equal
func CompareVersions(a, b string) int {
	arel, apre := versionParts(a)
	brel, bpre := versionParts(b)

	minlen := len(arel)
	if l := len(brel); l < minlen {
		minlen = l
	}

	// First compare major-minor-patch versions
	for i := 0; i < minlen; i++ {
		if arel[i] < brel[i] {
			return -1
		}
		if arel[i] > brel[i] {
			return 1
		}
	}

	// Longer version is newer, when the preceding parts are equal
	if len(arel) < len(brel) {
		return -1
	}
	if len(arel) > len(brel) {
		return 1
	}

	// Prerelease versions are older, if the versions are the same
	if len(apre) == 0 && len(bpre) > 0 {
		return 1
	}
	if len(apre) > 0 && len(bpre) == 0 {
		return -1
	}

	minlen = len(apre)
	if l := len(bpre); l < minlen {
		minlen = l
	}

	// Compare prerelease strings
	for i := 0; i < minlen; i++ {
		switch av := apre[i].(type) {
		case int:
			switch bv := bpre[i].(type) {
			case int:
				if av < bv {
					return -1
				}
				if av > bv {
					return 1
				}
			case string:
				return -1
			}
		case string:
			switch bv := bpre[i].(type) {
			case int:
				return 1
			case string:
				if av < bv {
					return -1
				}
				if av > bv {
					return 1
				}
			}
		}
	}

	// If all else is equal, longer prerelease string is newer
	if len(apre) < len(bpre) {
		return -1
	}
	if len(apre) > len(bpre) {
		return 1
	}

	// Looks like they're actually the same
	return 0
}

// Split a version into parts.
// "1.2.3-beta.2" -> []int{1, 2, 3}, []interface{}{"beta", 2}
func versionParts(v string) ([]int, []interface{}) {
	parts := strings.SplitN(v, "+", 2)
	parts = strings.SplitN(parts[0], "-", 2)
	fields := strings.Split(parts[0], ".")

	release := make([]int, len(fields))
	for i, s := range fields {
		v, _ := strconv.Atoi(s)
		release[i] = v
	}

	var prerelease []interface{}
	if len(parts) > 1 {
		fields = strings.Split(parts[1], ".")
		prerelease = make([]interface{}, len(fields))
		for i, s := range fields {
			v, err := strconv.Atoi(s)
			if err == nil {
				prerelease[i] = v
			} else {
				prerelease[i] = s
			}
		}
	}

	return release, prerelease
}

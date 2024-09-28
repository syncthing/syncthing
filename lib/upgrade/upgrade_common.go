// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package upgrade downloads and compares releases, and upgrades the running binary.
package upgrade

import (
	"errors"
	"fmt"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"

	"github.com/syncthing/syncthing/lib/build"
)

type Release struct {
	Tag        string  `json:"tag_name"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`

	// The HTML URL is needed for human readable links in the output created
	// by cmd/infra/stupgrades.
	HTMLURL string `json:"html_url"`

	// The compatibility information is included with each current release.
	Compatibility *ReleaseCompatibility `json:"compatibility,omitempty"`
}

type Asset struct {
	URL  string `json:"url"`
	Name string `json:"name"`

	// The browser URL is needed for human readable links in the output created
	// by cmd/infra/stupgrades.
	BrowserURL string `json:"browser_download_url,omitempty"`
}

// ReleaseCompatibility defines the structure of compat.json, which is
// included with each release.
type ReleaseCompatibility struct {
	Runtime      string            `json:"runtime,omitempty"`
	Requirements map[string]string `json:"requirements,omitempty"`
}

var (
	ErrNoReleaseDownload  = errors.New("couldn't find a release to download")
	ErrNoVersionToSelect  = errors.New("no version to select")
	ErrUpgradeUnsupported = errors.New("upgrade unsupported")
	ErrUpgradeInProgress  = errors.New("upgrade already in progress")
	upgradeUnlocked       = make(chan bool, 1)
)

func init() {
	upgradeUnlocked <- true
}

func To(rel Release) error {
	select {
	case <-upgradeUnlocked:
		path, err := os.Executable()
		if err != nil {
			upgradeUnlocked <- true
			return err
		}
		err = upgradeTo(path, rel)
		// If we've failed to upgrade, unlock so that another attempt could be made
		if err != nil {
			upgradeUnlocked <- true
		}
		return err
	default:
		return ErrUpgradeInProgress
	}
}

func ToURL(url string) error {
	select {
	case <-upgradeUnlocked:
		binary, err := os.Executable()
		if err != nil {
			upgradeUnlocked <- true
			return err
		}
		err = upgradeToURL(path.Base(url), binary, url)
		// If we've failed to upgrade, unlock so that another attempt could be made
		if err != nil {
			upgradeUnlocked <- true
		}
		return err
	default:
		return ErrUpgradeInProgress
	}
}

type Relation int

const (
	MajorOlder Relation = -2 // Older by a major version (x in x.y.z or 0.x.y).
	Older      Relation = -1 // Older by a minor version (y or z in x.y.z, or y in 0.x.y)
	Equal      Relation = 0  // Versions are semantically equal
	Newer      Relation = 1  // Newer by a minor version (y or z in x.y.z, or y in 0.x.y)
	MajorNewer Relation = 2  // Newer by a major version (x in x.y.z or 0.x.y).
)

// CompareVersions returns a relation describing how a compares to b.
func CompareVersions(a, b string) Relation {
	arel, apre := versionParts(a)
	brel, bpre := versionParts(b)

	minlen := len(arel)
	if l := len(brel); l < minlen {
		minlen = l
	}

	// First compare major-minor-patch versions
	for i := 0; i < minlen; i++ {
		if arel[i] < brel[i] {
			if i == 0 {
				// major version difference
				if arel[0] == 0 && brel[0] == 1 {
					// special case, v0.x is equivalent in majorness to v1.x.
					return Older
				}
				return MajorOlder
			}
			// minor or patch version difference
			return Older
		}
		if arel[i] > brel[i] {
			if i == 0 {
				// major version difference
				if arel[0] == 1 && brel[0] == 0 {
					// special case, v0.x is equivalent in majorness to v1.x.
					return Newer
				}
				return MajorNewer
			}
			// minor or patch version difference
			return Newer
		}
	}

	// Longer version is newer, when the preceding parts are equal
	if len(arel) < len(brel) {
		return Older
	}
	if len(arel) > len(brel) {
		return Newer
	}

	// Prerelease versions are older, if the versions are the same
	if len(apre) == 0 && len(bpre) > 0 {
		return Newer
	}
	if len(apre) > 0 && len(bpre) == 0 {
		return Older
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
					return Older
				}
				if av > bv {
					return Newer
				}
			case string:
				return Older
			}
		case string:
			switch bv := bpre[i].(type) {
			case int:
				return Newer
			case string:
				if av < bv {
					return Older
				}
				if av > bv {
					return Newer
				}
			}
		}
	}

	// If all else is equal, longer prerelease string is newer
	if len(apre) < len(bpre) {
		return Older
	}
	if len(apre) > len(bpre) {
		return Newer
	}

	// Looks like they're actually the same
	return Equal
}

// Split a version into parts.
// "1.2.3-beta.2" -> []int{1, 2, 3}, []interface{}{"beta", 2}
func versionParts(v string) ([]int, []interface{}) {
	if strings.HasPrefix(v, "v") || strings.HasPrefix(v, "V") {
		// Strip initial 'v' or 'V' prefix if present.
		v = v[1:]
	}
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

func releaseNames(tag string) []string {
	// We must ensure that the release asset matches the expected naming
	// standard, containing both the architecture/OS and the tag name we
	// expect. This protects against malformed release data potentially
	// tricking us into doing a downgrade.
	if build.IsDarwin {
		return []string{
			fmt.Sprintf("syncthing-macos-%s-%s.", runtime.GOARCH, tag),
			fmt.Sprintf("syncthing-macosx-%s-%s.", runtime.GOARCH, tag),
		}
	}
	return []string{
		fmt.Sprintf("syncthing-%s-%s-%s.", runtime.GOOS, runtime.GOARCH, tag),
	}
}

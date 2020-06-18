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
)

type Release struct {
	Tag        string  `json:"tag_name"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`

	// The HTML URL is needed for human readable links in the output created
	// by cmd/stupgrades.
	HTMLURL string `json:"html_url"`
}

type Asset struct {
	URL  string `json:"url"`
	Name string `json:"name"`

	// The browser URL is needed for human readable links in the output created
	// by cmd/stupgrades.
	BrowserURL string `json:"browser_download_url"`
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

// parsedVersion contains a version broken down into comparable parts. The
// release is assumed to be all digits and represented as such. The elements
// in prerelease and metadata parts are either integer or string. So, for
// v1.2.3-rc.4+22.g1234567-foo we get ...
type parsedVersion struct {
	release    []int         // 1, 2, 3
	prerelease []interface{} // "rc", 4
	metadata   []interface{} // 22, "g1234567-foo"
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
	ap := parseVersion(a)
	bp := parseVersion(b)

	if rel := compareReleaseDigits(ap.release, bp.release); rel != Equal {
		return rel
	}

	// If only one of the two versions is a prerelease, it's older. (Which
	// is different from the longer-is-newer default we would get by not
	// special-casing this.)
	if len(ap.prerelease) == 0 && len(bp.prerelease) > 0 {
		return Newer
	}
	if len(ap.prerelease) > 0 && len(bp.prerelease) == 0 {
		return Older
	}

	if rel := compareVariable(ap.prerelease, bp.prerelease); rel != Equal {
		return rel
	}

	return compareVariable(ap.metadata, bp.metadata)
}

func compareReleaseDigits(a, b []int) Relation {
	minlen := len(a)
	if l := len(b); l < minlen {
		minlen = l
	}

	// First compare major-minor-patch versions
	for i := 0; i < minlen; i++ {
		if a[i] < b[i] {
			if i == 0 {
				// major version difference
				if a[0] == 0 && b[0] == 1 {
					// special case, v0.x is equivalent in majorness to v1.x.
					return Older
				}
				return MajorOlder
			}
			// minor or patch version difference
			return Older
		}
		if a[i] > b[i] {
			if i == 0 {
				// major version difference
				if a[0] == 1 && b[0] == 0 {
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
	if len(a) < len(b) {
		return Older
	}
	if len(a) > len(b) {
		return Newer
	}

	return Equal
}

func compareVariable(a, b []interface{}) Relation {
	minlen := len(a)
	if l := len(b); l < minlen {
		minlen = l
	}

	for i := 0; i < minlen; i++ {
		switch ap := a[i].(type) {
		case string:
			switch bp := b[i].(type) {
			case string:
				if ap < bp {
					return Older
				}
				if ap > bp {
					return Newer
				}
			case int: // strings are newer than ints...
				return Newer
			}

		case int:
			switch bp := b[i].(type) {
			case string:
				return Older

			case int: // a and b are both ints
				if ap < bp {
					return Older
				}
				if ap > bp {
					return Newer
				}
			}
		}
	}

	// Longer is newer, when the preceding parts are equal
	if len(a) < len(b) {
		return Older
	}
	if len(a) > len(b) {
		return Newer
	}

	return Equal
}

func parseVersion(v string) parsedVersion {
	if strings.HasPrefix(v, "v") || strings.HasPrefix(v, "V") {
		// Strip initial 'v' or 'V' prefix if present.
		v = v[1:]
	}

	release, postPlus := splitOnFirst(v, "+")
	release, postDash := splitOnFirst(release, "-")

	return parsedVersion{
		release:    parseDigits(release),
		prerelease: parseVariable(postDash),
		metadata:   parseVariable(postPlus),
	}
}

func splitOnFirst(s, sep string) (string, string) {
	parts := strings.SplitN(s, sep, 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

func parseDigits(s string) []int {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ".")
	res := make([]int, len(parts))
	for i, str := range parts {
		res[i], _ = strconv.Atoi(str)
	}
	return res
}

func parseVariable(s string) []interface{} {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ".")
	res := make([]interface{}, len(parts))
	for i, str := range parts {
		num, err := strconv.Atoi(str)
		if err == nil {
			res[i] = num
		} else {
			res[i] = str
		}
	}
	return res
}

func releaseNames(tag string) []string {
	// We must ensure that the release asset matches the expected naming
	// standard, containing both the architecture/OS and the tag name we
	// expect. This protects against malformed release data potentially
	// tricking us into doing a downgrade.
	switch runtime.GOOS {
	case "darwin":
		return []string{
			fmt.Sprintf("syncthing-macos-%s-%s.", runtime.GOARCH, tag),
			fmt.Sprintf("syncthing-macosx-%s-%s.", runtime.GOARCH, tag),
		}
	default:
		return []string{
			fmt.Sprintf("syncthing-%s-%s-%s.", runtime.GOOS, runtime.GOARCH, tag),
		}
	}
}

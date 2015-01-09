// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

// Package upgrade downloads and compares releases, and upgrades the running binary.
package upgrade

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/calmh/osext"
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
func To(rel Release) error {
	select {
	case <-upgradeUnlocked:
		path, err := osext.Executable()
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

// A wrapper around actual implementations
func ToURL(url string) error {
	select {
	case <-upgradeUnlocked:
		path, err := osext.Executable()
		if err != nil {
			upgradeUnlocked <- true
			return err
		}
		err = upgradeToURL(path, url)
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
	Older               = -1 // Older by a minor version (y or z in x.y.z, or y in 0.x.y)
	Equal               = 0  // Versions are semantically equal
	Newer               = 1  // Newer by a minor version (y or z in x.y.z, or y in 0.x.y)
	MajorNewer          = 2  // Newer by a major version (x in x.y.z or 0.x.y).
)

// Returns a relation describing how a compares to b.
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
				return MajorOlder
			}
			if i == 1 && arel[0] == 0 {
				return MajorOlder
			}
			return Older
		}
		if arel[i] > brel[i] {
			if i == 0 {
				return MajorNewer
			}
			if i == 1 && arel[0] == 0 {
				return MajorNewer
			}
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

func releaseName(tag string) string {
	switch runtime.GOOS {
	case "darwin":
		return fmt.Sprintf("syncthing-macosx-%s-%s.", runtime.GOARCH, tag)
	default:
		return fmt.Sprintf("syncthing-%s-%s-%s.", runtime.GOOS, runtime.GOARCH, tag)
	}
}

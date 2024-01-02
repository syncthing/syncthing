// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package build

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const Codename = "Gold Grasshopper"

var (
	// Injected by build script
	Version = "unknown-dev"
	Host    = "unknown"
	User    = "unknown"
	Stamp   = "0"
	Tags    = ""

	// Set by init()
	Date        time.Time
	IsRelease   bool
	IsCandidate bool
	IsBeta      bool
	LongVersion string
	Extra       string

	allowedVersionExp = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[a-z0-9]+)*(\.\d+)*(\+\d+-g[0-9a-f]+|\+[0-9a-z]+)?(-[^\s]+)?$`)

	envTags = []string{
		"STGUIASSETS",
		"STHASHING",
		"STNORESTART",
		"STNOUPGRADE",
	}
)

const versionExtraAllowedChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-. "

func init() {
	if Version != "unknown-dev" {
		// If not a generic dev build, version string should come from git describe
		if !allowedVersionExp.MatchString(Version) {
			log.Fatalf("Invalid version string %q;\n\tdoes not match regexp %v", Version, allowedVersionExp)
		}
	}
	setBuildData()
}

func setBuildData() {
	// Check for a clean release build. A release is something like
	// "v0.1.2", with an optional suffix of letters and dot separated
	// numbers like "-beta3.47". If there's more stuff, like a plus sign and
	// a commit hash and so on, then it's not a release. If it has a dash in
	// it, it's some sort of beta, release candidate or special build. If it
	// has "-rc." in it, like "v0.14.35-rc.42", then it's a candidate build.
	//
	// So, every build that is not a stable release build has IsBeta = true.
	// This is used to enable some extra debugging (the deadlock detector).
	//
	// Release candidate builds are also "betas" from this point of view and
	// will have that debugging enabled. In addition, some features are
	// forced for release candidates - auto upgrade, and usage reporting.

	exp := regexp.MustCompile(`^v\d+\.\d+\.\d+(-[a-z]+[\d\.]+)?$`)
	IsRelease = exp.MatchString(Version)
	IsCandidate = strings.Contains(Version, "-rc.")
	IsBeta = strings.Contains(Version, "-")
	Extra = filterString(os.Getenv("STVERSIONEXTRA"), versionExtraAllowedChars)

	stamp, _ := strconv.Atoi(Stamp)
	Date = time.Unix(int64(stamp), 0)
	LongVersion = LongVersionFor("syncthing")
}

// LongVersionFor returns the long version string for the given program name.
func LongVersionFor(program string) string {
	// This string and date format is essentially part of our external API. Never change it.
	date := Date.UTC().Format("2006-01-02 15:04:05 MST")
	v := fmt.Sprintf(`%s %s "%s" (%s %s-%s) %s@%s %s`, program, Version, Codename, runtime.Version(), runtime.GOOS, runtime.GOARCH, User, Host, date)

	if tags := TagsList(); len(tags) > 0 {
		v = fmt.Sprintf("%s [%s]", v, strings.Join(tags, ", "))
	}
	return v
}

func TagsList() []string {
	tags := strings.Split(Tags, ",")
	if len(tags) == 1 && tags[0] == "" {
		tags = tags[:0]
	}
	for _, envVar := range envTags {
		if os.Getenv(envVar) != "" {
			tags = append(tags, strings.ToLower(envVar))
		}
	}
	if Extra != "" {
		tags = append(tags, Extra)
	}

	sort.Strings(tags)
	return tags
}

// filterString returns a copy of s with all characters not in allowedChars
// removed.
func filterString(s, allowedChars string) string {
	var res strings.Builder
	for _, c := range s {
		if strings.ContainsRune(allowedChars, c) {
			res.WriteRune(c)
		}
	}
	return res.String()
}

// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"encoding/json"
	"flag"
	"os"
	"sort"

	"github.com/syncthing/syncthing/lib/upgrade"
)

const defaultURL = "https://api.github.com/repos/syncthing/syncthing/releases?per_page=25"

func main() {
	url := flag.String("u", defaultURL, "GitHub releases url")
	flag.Parse()

	rels := upgrade.FetchLatestReleases(*url, "")
	if rels == nil {
		// An error was already logged
		os.Exit(1)
	}

	sort.Sort(upgrade.SortByRelease(rels))
	rels = filterForLatest(rels)

	json.NewEncoder(os.Stdout).Encode(rels)
}

// filterForLatest returns the latest stable and prerelease only
func filterForLatest(rels []upgrade.Release) []upgrade.Release {
	var filtered []upgrade.Release
	var havePre, haveStable bool
	for _, rel := range rels {
		if rel.Prerelease && !havePre {
			filtered = append(filtered, rel)
			havePre = true
		} else if !rel.Prerelease && !haveStable {
			filtered = append(filtered, rel)
			haveStable = true
		}
		if havePre && haveStable {
			break
		}
	}
	return filtered
}

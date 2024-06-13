// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package serve

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/syncthing/syncthing/cmd/ursrv/report"
	"github.com/syncthing/syncthing/lib/upgrade"
	"github.com/syncthing/syncthing/lib/ur"
)

type distributionMatch struct {
	matcher      *regexp.Regexp
	distribution string
}

var (
	progressBarClass   = []string{"", "progress-bar-success", "progress-bar-info", "progress-bar-warning", "progress-bar-danger"}
	blocksToGb         = float64(8 * 1024)
	plusStr            = "(+dev)"
	knownDistributions = []distributionMatch{
		// Maps well known builders to the official distribution method that
		// they represent

		{regexp.MustCompile(`\steamcity@build\.syncthing\.net`), "GitHub"},
		{regexp.MustCompile(`\sjenkins@build\.syncthing\.net`), "GitHub"},
		{regexp.MustCompile(`\sbuilder@github\.syncthing\.net`), "GitHub"},

		{regexp.MustCompile(`\sdeb@build\.syncthing\.net`), "APT"},
		{regexp.MustCompile(`\sdebian@github\.syncthing\.net`), "APT"},

		{regexp.MustCompile(`\sdocker@syncthing\.net`), "Docker Hub"},
		{regexp.MustCompile(`\sdocker@build.syncthing\.net`), "Docker Hub"},
		{regexp.MustCompile(`\sdocker@github.syncthing\.net`), "Docker Hub"},

		{regexp.MustCompile(`\sandroid-builder@github\.syncthing\.net`), "Google Play"},
		{regexp.MustCompile(`\sandroid-.*teamcity@build\.syncthing\.net`), "Google Play"},
		{regexp.MustCompile(`\sandroid-.*vagrant@basebox-stretch64`), "F-Droid"},
		{regexp.MustCompile(`\svagrant@bullseye`), "F-Droid"},
		{regexp.MustCompile(`\sbuilduser@(archlinux|svetlemodry)`), "Arch (3rd party)"},
		{regexp.MustCompile(`\ssyncthing@archlinux`), "Arch (3rd party)"},
		{regexp.MustCompile(`@debian`), "Debian (3rd party)"},
		{regexp.MustCompile(`@fedora`), "Fedora (3rd party)"},
		{regexp.MustCompile(`\sbrew@`), "Homebrew (3rd party)"},
		{regexp.MustCompile(`\sroot@buildkitsandbox`), "LinuxServer.io (3rd party)"},
		{regexp.MustCompile(`\sports@freebsd`), "FreeBSD (3rd party)"},
		{regexp.MustCompile(`.`), "Others"},
	}
)

// Functions used in index.html
var funcs = map[string]interface{}{
	"commatize":  commatize,
	"number":     number,
	"proportion": proportion,
	"counter": func() *counter {
		return &counter{}
	},
	"progressBarClassByIndex": func(a int) string {
		return progressBarClass[a%len(progressBarClass)]
	},
	"slice": func(numParts, whichPart int, input []report.Feature) []report.Feature {
		var part []report.Feature
		perPart := (len(input) / numParts) + len(input)%2

		parts := make([][]report.Feature, 0, numParts)
		for len(input) >= perPart {
			part, input = input[:perPart], input[perPart:]
			parts = append(parts, part)
		}
		if len(input) > 0 {
			parts = append(parts, input)
		}
		return parts[whichPart-1]
	},
}

func newBlockStats() [][]any {
	return [][]interface{}{
		{"Day", "Number of Reports", "Transferred (GiB)", "Saved by renaming files (GiB)", "Saved by resuming transfer (GiB)", "Saved by reusing data from old file (GiB)", "Saved by reusing shifted data from old file (GiB)", "Saved by reusing data from other files (GiB)"},
	}
}

func parseBlockStatsV2(rep ur.Aggregation, parsedDate string) []interface{} {
	// Legacy bad data on certain days
	if rep.Count <= 0 {
		return nil
	}

	// blockTotal, _ := intAnalysis(rep, "blockStats.total")
	blockRenamed, _, _ := intAnalysis(rep, "blockStats.renamed")
	blockReused, _, _ := intAnalysis(rep, "blockStats.reused")
	blockPulled, _, _ := intAnalysis(rep, "blockStats.pulled")
	blockCopyOrigin, _, _ := intAnalysis(rep, "blockStats.copyOrigin")
	blockCopyOriginShifted, _, _ := intAnalysis(rep, "blockStats.copyOriginShifted")
	blockElsewhere, _, _ := intAnalysis(rep, "blockStats.copyElsewhere")

	return []interface{}{
		parsedDate,
		rep.CountV3,
		float64(blockPulled.Sum) / blocksToGb,
		float64(blockRenamed.Sum) / blocksToGb,
		float64(blockReused.Sum) / blocksToGb,
		float64(blockCopyOrigin.Sum) / blocksToGb,
		float64(blockCopyOriginShifted.Sum) / blocksToGb,
		float64(blockElsewhere.Sum) / blocksToGb,
	}
}

func newPerformance() [][]interface{} {
	return [][]interface{}{
		{"Day", "TotFiles", "TotMiB", "SHA256Perf", "MemorySize", "MemoryUsageMiB"},
	}
}

type summary struct {
	versions map[string]int   // version string to count index
	max      map[string]int   // version string to max users per day
	rows     map[string][]int // date to list of counts
}

func newSummary() summary {
	return summary{
		versions: make(map[string]int),
		max:      make(map[string]int),
		rows:     make(map[string][]int),
	}
}

func simplifyVersion(version string) string {
	re := regexp.MustCompile(`^v\d.\d+`)
	return re.FindString(version)
}

func (s *summary) setCountsV2(date string, versions *ur.MapHistogram) {
	if versions == nil {
		return
	}

	simpleVersions := make(map[string]int64)
	for version, count := range versions.Map {
		version = simplifyVersion(version)
		if version == "" {
			continue
		}

		curr, ok := simpleVersions[version]
		if ok {
			count += curr
		}
		simpleVersions[version] = count
	}

	for version, count := range simpleVersions {
		if version == "v0.0" {
			// ?
			continue
		}

		// SUPER UGLY HACK to avoid having to do sorting properly
		if len(version) == 4 && strings.HasPrefix(version, "v0.") { // v0.x
			version = version[:3] + "0" + version[3:] // now v0.0x
		}

		s.setCount(date, version, int(count))
	}
}

func (s *summary) setCount(date, version string, count int) {
	idx, ok := s.versions[version]
	if !ok {
		idx = len(s.versions)
		s.versions[version] = idx
	}

	if s.max[version] < count {
		s.max[version] = count
	}

	row := s.rows[date]
	if len(row) <= idx {
		old := row
		row = make([]int, idx+1)
		copy(row, old)
		s.rows[date] = row
	}

	row[idx] = count

}

func (s *summary) MarshalJSON() ([]byte, error) {
	var versions []string
	for v := range s.versions {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(a, b int) bool {
		return upgrade.CompareVersions(versions[a], versions[b]) < 0
	})

	var filtered []string
	for _, v := range versions {
		if s.max[v] > 50 {
			filtered = append(filtered, v)
		}
	}
	versions = filtered

	headerRow := []interface{}{"Day"}
	for _, v := range versions {
		headerRow = append(headerRow, v)
	}

	var table [][]interface{}
	table = append(table, headerRow)

	var dates []string
	for k := range s.rows {
		dates = append(dates, k)
	}
	sort.Strings(dates)

	for _, date := range dates {
		row := []interface{}{date}
		for _, ver := range versions {
			idx := s.versions[ver]
			if len(s.rows[date]) > idx && s.rows[date][idx] > 0 {
				row = append(row, s.rows[date][idx])
			} else {
				row = append(row, nil)
			}
		}
		table = append(table, row)
	}

	return json.Marshal(table)
}

// filter removes versions that never reach the specified min count.
func (s *summary) filter(min int) {
	// We cheat and just remove the versions from the "index" and leave the
	// data points alone. The version index is used to build the table when
	// we do the serialization, so at that point the data points are
	// filtered out as well.
	for ver := range s.versions {
		if s.max[ver] < min {
			delete(s.versions, ver)
			delete(s.max, ver)
		}
	}
}

type sortableFeatureList []report.Feature

func (l sortableFeatureList) Len() int {
	return len(l)
}

func (l sortableFeatureList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}

func (l sortableFeatureList) Less(a, b int) bool {
	if l[a].Pct != l[b].Pct {
		return l[a].Pct < l[b].Pct
	}
	return l[a].Key > l[b].Key
}

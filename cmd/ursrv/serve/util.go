package serve

import (
	"encoding/json"
	"sort"

	"github.com/syncthing/syncthing/cmd/ursrv/report"
	"github.com/syncthing/syncthing/lib/upgrade"
)

var (
	progressBarClass = []string{"", "progress-bar-success", "progress-bar-info", "progress-bar-warning", "progress-bar-danger"}
	blocksToGb       = float64(8 * 1024)
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

func newBlockStats() [][]interface{} {
	return [][]interface{}{
		{"Day", "Number of Reports", "Transferred (GiB)", "Saved by renaming files (GiB)", "Saved by resuming transfer (GiB)", "Saved by reusing data from old file (GiB)", "Saved by reusing shifted data from old file (GiB)", "Saved by reusing data from other files (GiB)"},
	}
}

func parseBlockStats(date string, reports int, blockStats report.BlockStats) []interface{} {
	// Legacy bad data on certain days
	if reports <= 0 || !blockStats.Valid() {
		return nil
	}

	return []interface{}{
		date,
		reports,
		blockStats.Pulled / blocksToGb,
		blockStats.Renamed / blocksToGb,
		blockStats.Reused / blocksToGb,
		blockStats.CopyOrigin / blocksToGb,
		blockStats.CopyOriginShifted / blocksToGb,
		blockStats.CopyElsewhere / blocksToGb,
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

func (s *summary) setCount(date string, versions map[string]int) {
	for version, count := range versions {
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
}

func (s *summary) MarshalJSON() ([]byte, error) {
	var versions []string
	for v := range s.versions {
		versions = append(versions, v)
		println(v)
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

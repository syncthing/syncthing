// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package aggregate

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/syncthing/syncthing/cmd/ursrv/report"
)

type AnalyticList []report.Analytic

func (l AnalyticList) Less(a, b int) bool {
	if l[a].Key == "Others" {
		return false
	}
	if l[b].Key == "Others" {
		return true
	}
	return l[b].Count < l[a].Count // inverse
}

func (l AnalyticList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}

func (l AnalyticList) Len() int {
	return len(l)
}

// Returns a list of frequency Analytics for a given list of strings.
func analyticsFor(ss []string, cutoff int) []report.Analytic {
	m := make(map[string]int)
	t := 0
	for _, s := range ss {
		m[s]++
		t++
	}

	l := make([]report.Analytic, 0, len(m))
	for k, c := range m {
		l = append(l, report.Analytic{
			Key:        k,
			Count:      c,
			Percentage: 100 * float64(c) / float64(t),
		})
	}

	sort.Sort(AnalyticList(l))

	if cutoff > 0 && len(l) > cutoff {
		c := 0
		for _, i := range l[cutoff:] {
			c += i.Count
		}
		l = append(l[:cutoff], report.Analytic{
			Key:        "Others",
			Count:      c,
			Percentage: 100 * float64(c) / float64(t),
		})
	}

	return l
}

// Find the points at which certain penetration levels are met
func penetrationLevels(as []report.Analytic, points []float64) []report.Analytic {
	sort.Slice(as, func(a, b int) bool {
		return versionLess(as[b].Key, as[a].Key)
	})

	var res []report.Analytic

	idx := 0
	sum := 0.0
	for _, a := range as {
		sum += a.Percentage
		if sum >= points[idx] {
			a.Count = int(points[idx])
			a.Percentage = sum
			res = append(res, a)
			idx++
			if idx == len(points) {
				break
			}
		}
	}
	return res
}

func statsForInts(data []int) [4]float64 {
	var res [4]float64
	if len(data) == 0 {
		return res
	}

	sort.Ints(data)
	res[0] = float64(data[int(float64(len(data))*0.05)])
	res[1] = float64(data[len(data)/2])
	res[2] = float64(data[int(float64(len(data))*0.95)])
	res[3] = float64(data[len(data)-1])
	return res
}

func statsForInt64s(data []int64) [4]float64 {
	var res [4]float64
	if len(data) == 0 {
		return res
	}

	sort.Slice(data, func(a, b int) bool {
		return data[a] < data[b]
	})

	res[0] = float64(data[int(float64(len(data))*0.05)])
	res[1] = float64(data[len(data)/2])
	res[2] = float64(data[int(float64(len(data))*0.95)])
	res[3] = float64(data[len(data)-1])
	return res
}

func statsForFloats(data []float64) [4]float64 {
	var res [4]float64
	if len(data) == 0 {
		return res
	}

	sort.Float64s(data)
	res[0] = data[int(float64(len(data))*0.05)]
	res[1] = data[len(data)/2]
	res[2] = data[int(float64(len(data))*0.95)]
	res[3] = data[len(data)-1]
	return res
}

func group(by func(string) string, as []report.Analytic, perGroup int, otherPct float64) []report.Analytic {
	var res []report.Analytic

next:
	for _, a := range as {
		group := by(a.Key)
		for i := range res {
			if res[i].Key == group {
				res[i].Count += a.Count
				res[i].Percentage += a.Percentage
				if len(res[i].Items) < perGroup {
					res[i].Items = append(res[i].Items, a)
				}
				continue next
			}
		}
		res = append(res, report.Analytic{
			Key:        group,
			Count:      a.Count,
			Percentage: a.Percentage,
			Items:      []report.Analytic{a},
		})
	}

	sort.Sort(AnalyticList(res))

	if otherPct > 0 {
		// Groups with less than otherPCt go into "Other"
		other := report.Analytic{
			Key: "Other",
		}
		for i := 0; i < len(res); i++ {
			if res[i].Percentage < otherPct || res[i].Key == "Other" {
				other.Count += res[i].Count
				other.Percentage += res[i].Percentage
				res = append(res[:i], res[i+1:]...)
				i--
			}
		}
		if other.Count > 0 {
			res = append(res, other)
		}
	}

	return res
}

func byVersion(s string) string {
	parts := strings.Split(s, ".")
	if len(parts) >= 2 {
		return strings.Join(parts[:2], ".")
	}
	return s
}

func byPlatform(s string) string {
	parts := strings.Split(s, "-")
	if len(parts) >= 2 {
		return parts[0]
	}
	return s
}

var numericGoVersion = regexp.MustCompile(`^go[0-9]\.[0-9]+`)

func byCompiler(s string) string {
	if m := numericGoVersion.FindString(s); m != "" {
		return m
	}
	return "Other"
}

func versionLess(a, b string) bool {
	arel, apre := versionParts(a)
	brel, bpre := versionParts(b)

	minlen := len(arel)
	if l := len(brel); l < minlen {
		minlen = l
	}

	for i := 0; i < minlen; i++ {
		if arel[i] != brel[i] {
			return arel[i] < brel[i]
		}
	}

	// Longer version is newer, when the preceding parts are equal
	if len(arel) != len(brel) {
		return len(arel) < len(brel)
	}

	if apre != bpre {
		// "(+dev)" versions are ahead
		if apre == plusStr {
			return false
		}
		if bpre == plusStr {
			return true
		}
		return apre < bpre
	}

	// don't actually care how the prerelease stuff compares for our purposes
	return false
}

// Split a version as returned from transformVersion into parts.
// "1.2.3-beta.2" -> []int{1, 2, 3}, "beta.2"}
func versionParts(v string) ([]int, string) {
	parts := strings.SplitN(v[1:], " ", 2) // " (+dev)" versions
	if len(parts) == 1 {
		parts = strings.SplitN(parts[0], "-", 2) // "-rc.1" type versions
	}
	fields := strings.Split(parts[0], ".")

	release := make([]int, len(fields))
	for i, s := range fields {
		v, _ := strconv.Atoi(s)
		release[i] = v
	}

	var prerelease string
	if len(parts) > 1 {
		prerelease = parts[1]
	}

	return release, prerelease
}

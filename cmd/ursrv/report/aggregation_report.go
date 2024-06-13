// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package report

import (
	"fmt"
)

type AggregatedReport struct {
	// Date                time.Time                 `json:"date"`
	Features            map[string][]Feature      `json:"features"`
	FeatureGroups       map[string][]FeatureGroup `json:"featureGroups"`
	Nodes               int                       `json:"nodes"`
	VersionNodes        map[string]int64          `json:"versionNodes"`
	Categories          []Category                `json:"categories"`
	Versions            []Analytic                `json:"versions"`
	VersionPenetrations []Analytic                `json:"versionPenetrations"`
	Platforms           []Analytic                `json:"platforms"`
	Compilers           []Analytic                `json:"compilers"`
	Builders            []Analytic                `json:"builders"`
	Distributions       []Analytic                `json:"distributions"`
	FeatureOrder        []string                  `json:"featureOrder"`
	Locations           []Location                `json:"locations"`
	Countries           []Feature                 `json:"countries"`

	// VersionCount map[string]int `json:"versionCount"`
	// Performance  Performance    `json:"performance"`
	// BlockStats   BlockStats     `json:"blockStats"`
}

type Category struct {
	Values []float64  `json:"values"`
	Descr  string     `json:"descr"`
	Unit   string     `json:"unit,omitempty"`
	Type   NumberType `json:"type"`
}

type Feature struct {
	Key     string  `json:"key"`
	Version string  `json:"version"`
	Count   int     `json:"count"`
	Pct     float64 `json:"pct"`
}

type FeatureGroup struct {
	Key     string         `json:"key"`
	Version string         `json:"version"`
	Counts  map[string]int `json:"counts"`
}

func (fg *FeatureGroup) SumCounts() (sum int) {
	for _, value := range fg.Counts {
		sum += value
	}
	return
}

type Location struct {
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lon"`
	Count     int64   `json:"count"`
}

func (l *Location) Inc() {
	l.Count++
}

type Analytic struct {
	Key        string     `json:"key"`
	Count      int        `json:"count"`
	Percentage float64    `json:"percentage"`
	Children   []Analytic `json:"children,omitempty"`
}

type Performance struct {
	TotFiles       int     `json:"totFiles"`
	TotMib         int64   `json:"totMib"`
	Sha256Perf     float64 `json:"sha256Perf"`
	MemorySize     int64   `json:"memorySize"`
	MemoryUsageMib int64   `json:"memoryUsageMib"`
}

type BlockStats struct {
	NodeCount         int     `json:"nodeCount"`
	Total             float64 `json:"total"`
	Renamed           float64 `json:"renamed"`
	Reused            float64 `json:"reused"`
	Pulled            float64 `json:"pulled"`
	CopyOrigin        float64 `json:"copyOrigin"`
	CopyOriginShifted float64 `json:"copyOriginShifted"`
	CopyElsewhere     float64 `json:"copyElsewhere"`
}

func (b *BlockStats) Valid() bool {
	return b.Pulled >= 0 && b.Renamed >= 0 && b.Reused >= 0 && b.CopyOrigin >= 0 && b.CopyOriginShifted >= 0 && b.CopyElsewhere >= 0
}

type NumberType int

const (
	NumberMetric NumberType = iota
	NumberBinary
	NumberDuration
)

type LocationMap struct {
	mapped map[string]Location
}

func NewLocationsMap() LocationMap {
	return LocationMap{mapped: make(map[string]Location)}
}

func (lm *LocationMap) Add(lat, lon float64) {
	id := fmt.Sprintf("%g~%g", lat, lon)
	loc, ok := lm.mapped[id]
	if ok {
		loc.Inc()
	} else {
		loc = Location{Latitude: lat, Longitude: lon, Count: 1}
	}
	lm.mapped[id] = loc
}

func (lm *LocationMap) WeightedLocations() []Location {
	var locations []Location
	for _, location := range lm.mapped {
		locations = append(locations, location)
	}
	return locations
}

// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"fmt"
	"math"
)

var (
	privMin = 10
)

// Helper functions for private numbers and percentages
func isPrivate(n int) bool {
	return n < privMin
}

func privCount(x int) int {
	if isPrivate(x) {
		x = privMin
	}
	return x
}

func privCountString(n int) string {
	if isPrivate(n) {
		return fmt.Sprintf("< %d", privCount(n))
	}
	return fmt.Sprintf("%d", n)
}

func privCounts(x map[string]int) map[string]intString {
	fGroup := make(map[string]intString)
	for key, counts := range x {
		fGroup[key] = intString {
			privCount(counts),
			privCountString(counts),
		}
	}
	return fGroup
}

func privPct(x, total int) float64 {
	return (100 * float64(privCount(x))) / float64(total)
}

func privPctPct(x float64, private bool) float64 {
	if private {
		x *= 1.01
	}
	return x
}

func privPctString(n, total int) string {
	pct := privPct(n, total)
	return privPctPctString(pct, isPrivate(n))
}
func privPctPctString(pct float64, private bool) string {
	pct = privPctPct(pct, private)
	precision := int(math.Max(0, 2-math.Max(0, math.Log10(pct))))
	fmtPct := fmt.Sprintf("%%.0%df%%%%", precision)
	s := fmt.Sprintf(fmtPct, pct)
	if private {
		s = "< " + s
	}
	return s
}

// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package serve

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"

	"github.com/syncthing/syncthing/cmd/ursrv/report"
)

func number(ntype report.NumberType, v float64) string {
	switch ntype {
	case report.NumberMetric:
		return metric(v)
	case report.NumberDuration:
		return duration(v)
	case report.NumberBinary:
		return binary(v)
	default:
		return metric(v)
	}
}

type suffix struct {
	Suffix     string
	Multiplier float64
}

var metricSuffixes = []suffix{
	{"G", 1e9},
	{"M", 1e6},
	{"k", 1e3},
}

var binarySuffixes = []suffix{
	{"Gi", 1 << 30},
	{"Mi", 1 << 20},
	{"Ki", 1 << 10},
}

var durationSuffix = []suffix{
	{"year", 365 * 24 * 60 * 60},
	{"month", 30 * 24 * 60 * 60},
	{"day", 24 * 60 * 60},
	{"hour", 60 * 60},
	{"minute", 60},
	{"second", 1},
}

func metric(v float64) string {
	return withSuffix(v, metricSuffixes, false)
}

func binary(v float64) string {
	return withSuffix(v, binarySuffixes, false)
}

func duration(v float64) string {
	return withSuffix(v, durationSuffix, true)
}

func withSuffix(v float64, ps []suffix, pluralize bool) string {
	for _, p := range ps {
		if v >= p.Multiplier {
			suffix := p.Suffix
			if pluralize && v/p.Multiplier != 1.0 {
				suffix += "s"
			}
			// If the number only has decimal zeroes, strip em off.
			num := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", v/p.Multiplier), "0"), ".")
			return fmt.Sprintf("%s %s", num, suffix)
		}
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", v), "0"), ".")
}

// commatize returns a number with sep as thousands separators. Handles
// integers and plain floats.
func commatize(sep, s string) string {
	// If no dot, don't do anything.
	if !strings.ContainsRune(s, '.') {
		return s
	}
	var b bytes.Buffer
	fs := strings.SplitN(s, ".", 2)

	l := len(fs[0])
	for i := range fs[0] {
		b.Write([]byte{s[i]})
		if i < l-1 && (l-i)%3 == 1 {
			b.WriteString(sep)
		}
	}

	if len(fs) > 1 && len(fs[1]) > 0 {
		b.WriteString(".")
		b.WriteString(fs[1])
	}

	return b.String()
}

func proportion(m map[string]int, count int) float64 {
	total := 0
	isMax := true
	for _, n := range m {
		total += n
		if n > count {
			isMax = false
		}
	}
	pct := (100 * float64(count)) / float64(total)
	// To avoid rounding errors in the template, surpassing 100% and breaking
	// the progress bars.
	if isMax && len(m) > 1 && count != total {
		pct -= 0.01
	}
	return pct
}

func prettyCase(input string) string {
	output := ""
	for i, runeValue := range input {
		if i == 0 {
			runeValue = unicode.ToUpper(runeValue)
		} else if unicode.IsUpper(runeValue) {
			output += " "
		}
		output += string(runeValue)
	}
	return output
}

// Used in the templates
type counter struct {
	n int
}

func (c *counter) Current() int {
	return c.n
}

func (c *counter) Increment() string {
	c.n++
	return ""
}

func (c *counter) DrawTwoDivider() bool {
	return c.n != 0 && c.n%2 == 0
}

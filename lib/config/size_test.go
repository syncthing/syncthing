// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import "testing"

func TestParseSize(t *testing.T) {
	cases := []struct {
		in  string
		ok  bool
		val float64
		pct bool
	}{
		// We accept upper case SI units
		{"5K", true, 5e3, false}, // even when they should be lower case
		{"4 M", true, 4e6, false},
		{"3G", true, 3e9, false},
		{"2 T", true, 2e12, false},
		// We accept lower case SI units out of user friendliness
		{"1 k", true, 1e3, false},
		{"2m", true, 2e6, false},
		{"3 g", true, 3e9, false},
		{"4t", true, 4e12, false},
		// Fractions are OK
		{"123.456 k", true, 123.456e3, false},
		{"0.1234 m", true, 0.1234e6, false},
		{"3.45 g", true, 3.45e9, false},
		// We don't parse negative numbers
		{"-1", false, 0, false},
		{"-1k", false, 0, false},
		{"-0.45g", false, 0, false},
		// We accept various unit suffixes on the unit prefix
		{"100 KBytes", true, 100e3, false},
		{"100 Kbps", true, 100e3, false},
		{"100 MAU", true, 100e6, false},
		// Percentages are OK
		{"1%", true, 1, true},
		{"200%", true, 200, true},    // even large ones
		{"200K%", true, 200e3, true}, // even with prefixes, although this makes no sense
		{"2.34%", true, 2.34, true},  // fractions are A-ok
		// The empty string is a valid zero
		{"", true, 0, false},
		{"  ", true, 0, false},
	}

	for _, tc := range cases {
		size, err := ParseSize(tc.in)

		if !tc.ok {
			if err == nil {
				t.Errorf("Unexpected nil error in UnmarshalText(%q)", tc.in)
			}
			continue
		}

		if err != nil {
			t.Errorf("Unexpected error in UnmarshalText(%q): %v", tc.in, err)
			continue
		}
		if size.BaseValue() > tc.val*1.001 || size.BaseValue() < tc.val*0.999 {
			// Allow 0.1% slop due to floating point multiplication
			t.Errorf("Incorrect value in UnmarshalText(%q): %v, wanted %v", tc.in, size.BaseValue(), tc.val)
		}
		if size.Percentage() != tc.pct {
			t.Errorf("Incorrect percentage bool in UnmarshalText(%q): %v, wanted %v", tc.in, size.Percentage(), tc.pct)
		}
	}
}

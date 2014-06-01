// Copyright (C) 2014 Jakob Borg and other contributors. All rights reserved.
// Use of this source code is governed by an MIT-style license that can be
// found in the LICENSE file.

package main

import "testing"

var testcases = []struct {
	a, b string
	r    int
}{
	{"0.1.2", "0.1.2", 0},
	{"0.1.3", "0.1.2", 1},
	{"0.1.1", "0.1.2", -1},
	{"0.3.0", "0.1.2", 1},
	{"0.0.9", "0.1.2", -1},
	{"1.1.2", "0.1.2", 1},
	{"0.1.2", "1.1.2", -1},
	{"0.1.10", "0.1.9", 1},
	{"0.10.0", "0.2.0", 1},
	{"30.10.0", "4.9.0", 1},
}

func TestCompareVersions(t *testing.T) {
	for _, tc := range testcases {
		if r := compareVersions(tc.a, tc.b); r != tc.r {
			t.Errorf("compareVersions(%q, %q): %d != %d", tc.a, tc.b, r, tc.r)
		}
	}
}

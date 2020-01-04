// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package backend

import "testing"

func TestCommonPrefix(t *testing.T) {
	cases := []struct {
		a      string
		b      string
		common string
	}{
		{"", "", ""},
		{"a", "b", ""},
		{"aa", "ab", "a"},
		{"aa", "a", "a"},
		{"a", "aa", "a"},
		{"aabab", "ab", "a"},
		{"ab", "aabab", "a"},
		{"abac", "ababab", "aba"},
		{"ababab", "abac", "aba"},
	}

	for _, tc := range cases {
		pref := string(commonPrefix([]byte(tc.a), []byte(tc.b)))
		if pref != tc.common {
			t.Errorf("commonPrefix(%q, %q) => %q, expected %q", tc.a, tc.b, pref, tc.common)
		}
	}
}

func TestBadgerBackendBehavior(t *testing.T) {
	testBackendBehavior(t, OpenBadgerMemory)
}

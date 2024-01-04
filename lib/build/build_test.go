// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package build

import (
	"testing"
)

func TestAllowedVersions(t *testing.T) {
	testcases := []struct {
		ver     string
		allowed bool
	}{
		{"v0.13.0", true},
		{"v0.12.11+22-gabcdef0", true},
		{"v0.13.0-beta0", true},
		{"v0.13.0-beta47", true},
		{"v0.13.0-beta47+1-gabcdef0", true},
		{"v0.13.0-beta.0", true},
		{"v0.13.0-beta.47", true},
		{"v0.13.0-beta.0+1-gabcdef0", true},
		{"v0.13.0-beta.47+1-gabcdef0", true},
		{"v0.13.0-some-weird-but-allowed-tag", true},
		{"v0.13.0-allowed.to.do.this", true},
		{"v0.13.0+not.allowed.to.do.this", false},
		{"v1.27.0+xyz", true},
		{"v1.27.0-abc.1+xyz", true},
		{"v1.0.0+45", true},
		{"v1.0.0-noupgrade", true},
		{"v1.0.0+noupgrade", true},
	}

	for i, c := range testcases {
		if allowed := AllowedVersionExp.MatchString(c.ver); allowed != c.allowed {
			t.Errorf("%d: incorrect result %v != %v for %q", i, allowed, c.allowed, c.ver)
		}
	}
}

func TestFilterString(t *testing.T) {
	cases := []struct {
		input  string
		filter string
		output string
	}{
		{"abcba", "abc", "abcba"},
		{"abcba", "ab", "abba"},
		{"abcba", "c", "c"},
		{"abcba", "!", ""},
		{"Foo (v1.5)", versionExtraAllowedChars, "Foo v1.5"},
	}

	for i, c := range cases {
		if out := filterString(c.input, c.filter); out != c.output {
			t.Errorf("%d: %q != %q", i, out, c.output)
		}
	}
}

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
		/* original test samples */
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
		{"v1.27.0+xyz", true},
		{"v1.27.0-abc.1+xyz", true},
		{"v1.0.0+45", true},
		{"v1.0.0-noupgrade", true},
		{"v1.0.0+noupgrade", true},
		/* samples from semver.org (https://regex101.com/r/vkijKf/1/) */
		{"v0.0.4", true},
		{"v1.2.3", true},
		{"v10.20.30", true},
		{"v1.1.2-prerelease+meta", true},
		{"v1.1.2+meta", true},
		{"v1.1.2+meta-valid", true},
		{"v1.0.0-alpha", true},
		{"v1.0.0-beta", true},
		{"v1.0.0-alpha.beta", true},
		{"v1.0.0-alpha.beta.1", true},
		{"v1.0.0-alpha.1", true},
		{"v1.0.0-alpha0.valid", true},
		{"v1.0.0-alpha.0valid", true},
		{"v1.0.0-alpha-a.b-c-somethinglong+build.1-aef.1-its-okay", true},
		{"v1.0.0-rc.1+build.1", true},
		{"v2.0.0-rc.1+build.123", true},
		{"v1.2.3-beta", true},
		{"v10.2.3-DEV-SNAPSHOT", true},
		{"v1.2.3-SNAPSHOT-123", true},
		{"v1.0.0", true},
		{"v2.0.0", true},
		{"v1.1.7", true},
		{"v2.0.0+build.1848", true},
		{"v2.0.1-alpha.1227", true},
		{"v1.0.0-alpha+beta", true},
		{"v1.2.3----RC-SNAPSHOT.12.9.1--.12+788", true},
		{"v1.2.3----R-S.12.9.1--.12+meta", true},
		{"v1.2.3----RC-SNAPSHOT.12.9.1--.12", true},
		{"v1.0.0+0.build.1-rc.10000aaa-kk-0.1", true},
		{"v99999999999999999999999.999999999999999999.99999999999999999", true},
		// {"1.0.0-0A.is.legal", true} doesn't work with provided regex but is valid
		{"v1", false},
		{"v1.2", false},
		{"v1.2.3-0123", false},
		{"v1.2.3-0123.0123", false},
		{"v1.1.2+.123", false},
		{"v+invalid", false},
		{"v-invalid", false},
		{"v-invalid+invalid", false},
		{"v-invalid.01", false},
		{"valpha", false},
		{"valpha.beta", false},
		{"valpha.beta.1", false},
		{"valpha.1", false},
		{"valpha+beta", false},
		{"valpha_beta", false},
		{"valpha.", false},
		{"valpha..", false},
		{"vbeta", false},
		{"v1.0.0-alpha_beta", false},
		{"v-alpha.", false},
		{"v1.0.0-alpha..", false},
		{"v1.0.0-alpha..1", false},
		{"v1.0.0-alpha...1", false},
		{"v1.0.0-alpha....1", false},
		{"v1.0.0-alpha.....1", false},
		{"v1.0.0-alpha......1", false},
		{"v1.0.0-alpha.......1", false},
		{"v01.1.1", false},
		{"v1.01.1", false},
		{"v1.1.01", false},
		{"v1.2", false},
		{"v1.2.3.DEV", false},
		{"v1.2-SNAPSHOT", false},
		{"v1.2.31.2.3----RC-SNAPSHOT.12.09.1--..12+788", false},
		{"v1.2-RC-SNAPSHOT", false},
		{"v-1.0.3-gamma+b7718", false},
		{"v+justmeta", false},
		{"v9.8.7+meta+meta", false},
		{"v9.8.7-whatever+meta+meta", false},
		{"v99999999999999999999999.999999999999999999.99999999999999999----RC-SNAPSHOT.12.09.1--------------------------------..12", false},
	}

	for i, c := range testcases {
		if allowed := allowedVersionExp.MatchString(c.ver); allowed != c.allowed {
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

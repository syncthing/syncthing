// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package ignoreresult_test

import (
	"testing"

	"github.com/syncthing/syncthing/lib/ignore/ignoreresult"
)

func TestFlagCanSkipDir(t *testing.T) {
	// Verify that CanSkipDir() means that something is both ignored and can
	// be skipped as a directory, so that it's legitimate to say
	// Match(...).CanSkipDir() instead of having to create a temporary
	// variable and check both Match(...).IsIgnored() and
	// Match(...).CanSkipDir().

	cases := []struct {
		res        ignoreresult.R
		canSkipDir bool
	}{
		{0, false},
		{ignoreresult.NotIgnored, false},
		{ignoreresult.NotIgnored.WithSkipDir(), false},
		{ignoreresult.Ignored, false},
		{ignoreresult.IgnoreAndSkip, true},
	}

	for _, tc := range cases {
		if tc.res.CanSkipDir() != tc.canSkipDir {
			t.Errorf("%v.CanSkipDir() != %v", tc.res, tc.canSkipDir)
		}
	}
}

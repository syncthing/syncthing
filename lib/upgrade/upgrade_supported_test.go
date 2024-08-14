// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !noupgrade && !ios
// +build !noupgrade,!ios

package upgrade

import (
	"os"
	"testing"
)

func TestFetchLatestReleases(t *testing.T) {
	releasesURL := os.Getenv(testingReleasesURL)
	if releasesURL == "" {
		t.Skipf("Skipping as %q is not set", testingReleasesURL)
	}

	t.Logf("Calling FetchLatestReleases(%q, %q)", releasesURL, "")
	rel := FetchLatestReleases(releasesURL, "")
	if rel == nil {
		t.Errorf("FetchLatestReleases(%q, %q): got nil, want not-nil", releasesURL, "")
	}
}

func TestLatestRelease(t *testing.T) {
	releasesURL := os.Getenv(testingReleasesURL)
	if releasesURL == "" {
		t.Skipf("Skipping as %q is not set", testingReleasesURL)
	}

	for _, upgradeToPreReleases := range []bool{false, true} {
		t.Logf("Calling LatestRelease(%q, %q, %v)", releasesURL, "", upgradeToPreReleases)
		_, err := LatestRelease(releasesURL, "", upgradeToPreReleases)
		if err != nil {
			t.Errorf("LatestRelease(%q, %q, %v): got error %q, want nil", releasesURL, "", upgradeToPreReleases, err)
		}
	}
}

// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build noupgrade
// +build noupgrade

package upgrade

const DisabledByCompilation = true

func upgradeTo(binary string, rel Release) error {
	return ErrUpgradeUnsupported
}

func upgradeToURL(archiveName, binary, url string) error {
	return ErrUpgradeUnsupported
}

func LatestRelease(releasesURL, current string, upgradeToPreRelease bool) (Release, error) {
	return Release{}, ErrUpgradeUnsupported
}

// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build solaris noupgrade

package upgrade

func UpgradeTo(rel Release, extra string) error {
	return ErrUpgradeUnsupported
}

func LatestRelease(prerelease bool) (Release, error) {
	return Release{}, ErrUpgradeUnsupported
}

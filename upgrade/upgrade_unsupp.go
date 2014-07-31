// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build windows solaris noupgrade

package upgrade

func Upgrade(current string) (Release, error) {
	return Release{}, ErrUpgradeUnsupported
}

func LatestRelease(prerelease bool) (Release, error) {
	return Release{}, ErrUpgradeUnsupported
}

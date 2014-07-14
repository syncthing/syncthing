// +build windows solaris noupgrade

package main

import "errors"

var errUpgradeUnsupported = errors.New("Automatic upgrade not supported")

func upgrade() error {
	return errUpgradeUnsupported
}

func currentRelease() (githubRelease, error) {
	return githubRelease{}, errUpgradeUnsupported
}

// +build windows solaris noupgrade

package main

func upgrade() error {
	return errUpgradeUnsupported
}

func currentRelease() (githubRelease, error) {
	return githubRelease{}, errUpgradeUnsupported
}

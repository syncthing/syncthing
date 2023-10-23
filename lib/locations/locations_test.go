// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !windows

package locations

import (
	"testing"

	"golang.org/x/exp/slices"
)

func TestUnixConfigDir(t *testing.T) {
	t.Parallel()

	cases := []struct {
		userHome      string
		xdgConfigHome string
		xdgStateHome  string
		filesExist    []string
		expected      string
	}{
		// First some "new installations", no files exist previously.

		// No variables set, use our current default
		{"/home/user", "", "", nil, "/home/user/.local/state/syncthing"},
		// Config home set, doesn't matter
		{"/home/user", "/somewhere/else", "", nil, "/home/user/.local/state/syncthing"},
		// State home set, use that
		{"/home/user", "", "/var/state", nil, "/var/state/syncthing"},
		// State home set, again config home doesn't matter
		{"/home/user", "/somewhere/else", "/var/state", nil, "/var/state/syncthing"},

		// Now some "upgrades", where we have files in the old locations.

		// Config home set, a file exists in the default location
		{"/home/user", "/somewhere/else", "", []string{"/home/user/.config/syncthing/config.xml"}, "/home/user/.config/syncthing"},
		// State home set, a file exists in the default location
		{"/home/user", "", "/var/state", []string{"/home/user/.config/syncthing/config.xml"}, "/home/user/.config/syncthing"},
		// Both config home and state home set, a file exists in the default location
		{"/home/user", "/somewhere/else", "/var/state", []string{"/home/user/.config/syncthing/config.xml"}, "/home/user/.config/syncthing"},

		// Config home set, and a file exists at that place
		{"/home/user", "/somewhere/else", "", []string{"/somewhere/else/syncthing/config.xml"}, "/somewhere/else/syncthing"},
		// Config home and state home set, and a file exists in config home
		{"/home/user", "/somewhere/else", "/var/state", []string{"/somewhere/else/syncthing/config.xml"}, "/somewhere/else/syncthing"},
	}

	for _, c := range cases {
		fileExists := func(path string) bool { return slices.Contains(c.filesExist, path) }
		actual := unixConfigDir(c.userHome, c.xdgConfigHome, c.xdgStateHome, fileExists)
		if actual != c.expected {
			t.Errorf("unixConfigDir(%q, %q, %q) == %q, expected %q", c.userHome, c.xdgConfigHome, c.xdgStateHome, actual, c.expected)
		}
	}
}

func TestUnixDataDir(t *testing.T) {
	t.Parallel()

	cases := []struct {
		userHome     string
		configDir    string
		xdgDataHome  string
		xdgStateHome string
		filesExist   []string
		expected     string
	}{
		// First some "new installations", no files exist previously.

		// No variables set, use our current default
		{"/home/user", "", "", "", nil, "/home/user/.local/state/syncthing"},
		// Data home set, doesn't matter
		{"/home/user", "", "/somewhere/else", "", nil, "/home/user/.local/state/syncthing"},
		// State home set, use that
		{"/home/user", "", "", "/var/state", nil, "/var/state/syncthing"},

		// Now some "upgrades", where we have files in the old locations.

		// A database exists in the old default location, use that
		{"/home/user", "", "", "", []string{"/home/user/.config/syncthing/index-v0.14.0.db"}, "/home/user/.config/syncthing"},
		{"/home/user", "/config/dir", "/xdg/data/home", "/xdg/state/home", []string{"/home/user/.config/syncthing/index-v0.14.0.db"}, "/home/user/.config/syncthing"},

		// A database exists in the config dir, use that
		{"/home/user", "/config/dir", "/xdg/data/home", "/xdg/state/home", []string{"/config/dir/index-v0.14.0.db"}, "/config/dir"},

		// A database exists in the old xdg data home, use that
		{"/home/user", "/config/dir", "/xdg/data/home", "/xdg/state/home", []string{"/xdg/data/home/syncthing/index-v0.14.0.db"}, "/xdg/data/home/syncthing"},
	}

	for _, c := range cases {
		fileExists := func(path string) bool { return slices.Contains(c.filesExist, path) }
		actual := unixDataDir(c.userHome, c.configDir, c.xdgDataHome, c.xdgStateHome, fileExists)
		if actual != c.expected {
			t.Errorf("unixDataDir(%q, %q, %q, %q) == %q, expected %q", c.userHome, c.configDir, c.xdgDataHome, c.xdgStateHome, actual, c.expected)
		}
	}
}

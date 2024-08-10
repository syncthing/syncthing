// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !windows

package locations

import (
	"path/filepath"
	"slices"
	"testing"
	"time"
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
		{"/home/user", "", "", nil, "/home/user/.local/state/syncthing_ofuse"},
		// Config home set, doesn't matter
		{"/home/user", "/somewhere/else", "", nil, "/home/user/.local/state/syncthing_ofuse"},
		// State home set, use that
		{"/home/user", "", "/var/state", nil, "/var/state/syncthing_ofuse"},
		// State home set, again config home doesn't matter
		{"/home/user", "/somewhere/else", "/var/state", nil, "/var/state/syncthing_ofuse"},

		// Now some "upgrades", where we have files in the old locations.

		// Config home set, a file exists in the default location
		{"/home/user", "/somewhere/else", "", []string{"/home/user/.config/syncthing_ofuse/config.xml"}, "/home/user/.config/syncthing_ofuse"},
		// State home set, a file exists in the default location
		{"/home/user", "", "/var/state", []string{"/home/user/.config/syncthing_ofuse/config.xml"}, "/home/user/.config/syncthing_ofuse"},
		// Both config home and state home set, a file exists in the default location
		{"/home/user", "/somewhere/else", "/var/state", []string{"/home/user/.config/syncthing_ofuse/config.xml"}, "/home/user/.config/syncthing_ofuse"},

		// Config home set, and a file exists at that place
		{"/home/user", "/somewhere/else", "", []string{"/somewhere/else/syncthing_ofuse/config.xml"}, "/somewhere/else/syncthing_ofuse"},
		// Config home and state home set, and a file exists in config home
		{"/home/user", "/somewhere/else", "/var/state", []string{"/somewhere/else/syncthing_ofuse/config.xml"}, "/somewhere/else/syncthing_ofuse"},
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
		{"/home/user", "", "", "", nil, "/home/user/.local/state/syncthing_ofuse"},
		// Data home set, doesn't matter
		{"/home/user", "", "/somewhere/else", "", nil, "/home/user/.local/state/syncthing_ofuse"},
		// State home set, use that
		{"/home/user", "", "", "/var/state", nil, "/var/state/syncthing_ofuse"},

		// Now some "upgrades", where we have files in the old locations.

		// A database exists in the old default location, use that
		{"/home/user", "", "", "", []string{"/home/user/.config/syncthing_ofuse/index-v0.14.0.db"}, "/home/user/.config/syncthing_ofuse"},
		{"/home/user", "/config/dir", "/xdg/data/home", "/xdg/state/home", []string{"/home/user/.config/syncthing_ofuse/index-v0.14.0.db"}, "/home/user/.config/syncthing_ofuse"},

		// A database exists in the config dir, use that
		{"/home/user", "/config/dir", "/xdg/data/home", "/xdg/state/home", []string{"/config/dir/index-v0.14.0.db"}, "/config/dir"},

		// A database exists in the old xdg data home, use that
		{"/home/user", "/config/dir", "/xdg/data/home", "/xdg/state/home", []string{"/xdg/data/home/syncthing_ofuse/index-v0.14.0.db"}, "/xdg/data/home/syncthing_ofuse"},
	}

	for _, c := range cases {
		fileExists := func(path string) bool { return slices.Contains(c.filesExist, path) }
		actual := unixDataDir(c.userHome, c.configDir, c.xdgDataHome, c.xdgStateHome, fileExists)
		if actual != c.expected {
			t.Errorf("unixDataDir(%q, %q, %q, %q) == %q, expected %q", c.userHome, c.configDir, c.xdgDataHome, c.xdgStateHome, actual, c.expected)
		}
	}
}

func TestGetTimestamped(t *testing.T) {
	s := getTimestampedAt(PanicLog, time.Date(2023, 10, 25, 21, 47, 0, 0, time.UTC))
	exp := "panic-20231025-214700.log"
	if file := filepath.Base(s); file != exp {
		t.Errorf("got %q, expected %q", file, exp)
	}
}

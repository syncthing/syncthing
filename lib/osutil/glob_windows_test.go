// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build windows

package osutil_test

import (
	"testing"

	"github.com/syncthing/syncthing/lib/osutil"
)

func TestGlob(t *testing.T) {
	testcases := []string{
		`C:\*`,
		`\\?\C:\*`,
		`\\?\C:\Users`,
		`\\?\\\?\C:\Users`,
	}
	for _, tc := range testcases {
		if _, err := osutil.Glob(tc); err != nil {
			t.Fatalf("pattern %s failed: %v", tc, err)
		}
	}
}

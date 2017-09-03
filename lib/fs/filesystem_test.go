// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"path/filepath"
	"testing"
)

func TestIsInternal(t *testing.T) {
	cases := []struct {
		file     string
		internal bool
	}{
		{".stfolder", true},
		{".stignore", true},
		{".stversions", true},
		{".stfolder/foo", true},
		{".stignore/foo", true},
		{".stversions/foo", true},

		{".stfolderfoo", false},
		{".stignorefoo", false},
		{".stversionsfoo", false},
		{"foo.stfolder", false},
		{"foo.stignore", false},
		{"foo.stversions", false},
		{"foo/.stfolder", false},
		{"foo/.stignore", false},
		{"foo/.stversions", false},
	}

	for _, tc := range cases {
		res := IsInternal(filepath.FromSlash(tc.file))
		if res != tc.internal {
			t.Errorf("Unexpected result: IsInteral(%q): %v should be %v", tc.file, res, tc.internal)
		}
	}
}

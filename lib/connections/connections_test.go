// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package connections

import "testing"
import "net/url"

func TestFixupPort(t *testing.T) {
	cases := [][2]string{
		{"tcp://1.2.3.4:5", "tcp://1.2.3.4:5"},
		{"tcp://1.2.3.4:", "tcp://1.2.3.4:22000"},
		{"tcp://1.2.3.4", "tcp://1.2.3.4:22000"},
	}

	for _, tc := range cases {
		u0, _ := url.Parse(tc[0])
		u1 := fixupPort(u0).String()
		if u1 != tc[1] {
			t.Errorf("fixupPort(%q) => %q, expected %q", tc[0], u1, tc[1])
		}
	}
}

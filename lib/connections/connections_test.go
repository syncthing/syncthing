// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

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
		u1 := fixupPort(u0, 22000).String()
		if u1 != tc[1] {
			t.Errorf("fixupPort(%q, 22000) => %q, expected %q", tc[0], u1, tc[1])
		}
	}
}

func TestAllowedNetworks(t *testing.T) {
	cases := []struct {
		host    string
		allowed []string
		ok      bool
	}{
		{
			"192.168.0.1",
			nil,
			false,
		},
		{
			"192.168.0.1",
			[]string{},
			false,
		},
		{
			"fe80::1",
			nil,
			false,
		},
		{
			"fe80::1",
			[]string{},
			false,
		},
		{
			"192.168.0.1",
			[]string{"fe80::/48", "192.168.0.0/24"},
			true,
		},
		{
			"fe80::1",
			[]string{"192.168.0.0/24", "fe80::/48"},
			true,
		},
		{
			"192.168.0.1",
			[]string{"192.168.1.0/24", "fe80::/48"},
			false,
		},
		{
			"fe80::1",
			[]string{"fe82::/48", "192.168.1.0/24"},
			false,
		},
		{
			"192.168.0.1:4242",
			[]string{"fe80::/48", "192.168.0.0/24"},
			true,
		},
		{
			"[fe80::1]:4242",
			[]string{"192.168.0.0/24", "fe80::/48"},
			true,
		},
		{
			"10.20.30.40",
			[]string{"!10.20.30.0/24", "10.0.0.0/8"},
			false,
		},
		{
			"10.20.30.40",
			[]string{"10.0.0.0/8", "!10.20.30.0/24"},
			true,
		},
		{
			"[fe80::1]:4242",
			[]string{"192.168.0.0/24", "!fe00::/8", "fe80::/48"},
			false,
		},
	}

	for _, tc := range cases {
		res := IsAllowedNetwork(tc.host, tc.allowed)
		if res != tc.ok {
			t.Errorf("allowedNetwork(%q, %q) == %v, want %v", tc.host, tc.allowed, res, tc.ok)
		}
	}
}

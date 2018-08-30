// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"fmt"
	"net"
	"testing"
)

func TestFixupAddresses(t *testing.T) {
	cases := []struct {
		remote net.IP
		in     []string
		out    []string
	}{
		{ // verbatim passthrough
			in:  []string{"tcp://1.2.3.4:22000"},
			out: []string{"tcp://1.2.3.4:22000"},
		}, { // unspecified replaced by remote
			remote: net.ParseIP("1.2.3.4"),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://1.2.3.4:22000", "tcp://192.0.2.42:22000"},
		}, { // unspecified not used as replacement
			remote: net.ParseIP("0.0.0.0"),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		}, { // unspecified not used as replacement
			remote: net.ParseIP("::"),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		}, { // localhost not used as replacement
			remote: net.ParseIP("127.0.0.1"),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		}, { // localhost not used as replacement
			remote: net.ParseIP("::1"),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		}, { // multicast not used as replacement
			remote: net.ParseIP("224.0.0.1"),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		}, { // multicast not used as replacement
			remote: net.ParseIP("ff80::42"),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		}, { // explicitly announced weirdness is also filtered
			remote: net.ParseIP("192.0.2.42"),
			in:     []string{"tcp://:22000", "tcp://127.1.2.3:22000", "tcp://[::1]:22000", "tcp://[ff80::42]:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		},
	}

	for _, tc := range cases {
		out := fixupAddresses(tc.remote, tc.in)
		if fmt.Sprint(out) != fmt.Sprint(tc.out) {
			t.Errorf("fixupAddresses(%v, %v) => %v, expected %v", tc.remote, tc.in, out, tc.out)
		}
	}
}

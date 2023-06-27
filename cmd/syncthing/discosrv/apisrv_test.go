// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package discosrv

import (
	"fmt"
	"net"
	"testing"
)

func TestFixupAddresses(t *testing.T) {
	cases := []struct {
		remote *net.TCPAddr
		in     []string
		out    []string
	}{
		{ // verbatim passthrough
			in:  []string{"tcp://1.2.3.4:22000"},
			out: []string{"tcp://1.2.3.4:22000"},
		}, { // unspecified replaced by remote
			remote: addr("1.2.3.4", 22000),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://1.2.3.4:22000", "tcp://192.0.2.42:22000"},
		}, { // unspecified not used as replacement
			remote: addr("0.0.0.0", 22000),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		}, { // unspecified not used as replacement
			remote: addr("::", 22000),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		}, { // localhost not used as replacement
			remote: addr("127.0.0.1", 22000),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		}, { // localhost not used as replacement
			remote: addr("::1", 22000),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		}, { // multicast not used as replacement
			remote: addr("224.0.0.1", 22000),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		}, { // multicast not used as replacement
			remote: addr("ff80::42", 22000),
			in:     []string{"tcp://:22000", "tcp://192.0.2.42:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		}, { // explicitly announced weirdness is also filtered
			remote: addr("192.0.2.42", 22000),
			in:     []string{"tcp://:22000", "tcp://127.1.2.3:22000", "tcp://[::1]:22000", "tcp://[ff80::42]:22000"},
			out:    []string{"tcp://192.0.2.42:22000"},
		}, { // port remapping
			remote: addr("123.123.123.123", 9000),
			in:     []string{"tcp://0.0.0.0:0"},
			out:    []string{"tcp://123.123.123.123:9000"},
		}, { // unspecified port remapping
			remote: addr("123.123.123.123", 9000),
			in:     []string{"tcp://:0"},
			out:    []string{"tcp://123.123.123.123:9000"},
		}, { // empty remapping
			remote: addr("123.123.123.123", 9000),
			in:     []string{"tcp://"},
			out:    []string{},
		}, { // port only remapping
			remote: addr("123.123.123.123", 9000),
			in:     []string{"tcp://44.44.44.44:0"},
			out:    []string{"tcp://44.44.44.44:9000"},
		},
	}

	for _, tc := range cases {
		out := fixupAddresses(tc.remote, tc.in)
		if fmt.Sprint(out) != fmt.Sprint(tc.out) {
			t.Errorf("fixupAddresses(%v, %v) => %v, expected %v", tc.remote, tc.in, out, tc.out)
		}
	}
}

func addr(host string, port int) *net.TCPAddr {
	return &net.TCPAddr{
		IP:   net.ParseIP(host),
		Port: port,
	}
}

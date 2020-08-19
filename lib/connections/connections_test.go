// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestFixupPort(t *testing.T) {
	cases := [][2]string{
		{"tcp://1.2.3.4:5", "tcp://1.2.3.4:5"},
		{"tcp://1.2.3.4:", "tcp://1.2.3.4:22000"},
		{"tcp://1.2.3.4", "tcp://1.2.3.4:22000"},
		{"tcp://[fe80::1]", "tcp://[fe80::1]:22000"},
		{"tcp://[fe80::1]:", "tcp://[fe80::1]:22000"},
		{"tcp://[fe80::1]:22000", "tcp://[fe80::1]:22000"},
		{"tcp://[fe80::1]:22000", "tcp://[fe80::1]:22000"},
		{"tcp://[fe80::1%25abc]", "tcp://[fe80::1%25abc]:22000"},
		{"tcp://[fe80::1%25abc]:", "tcp://[fe80::1%25abc]:22000"},
		{"tcp://[fe80::1%25abc]:22000", "tcp://[fe80::1%25abc]:22000"},
		{"tcp://[fe80::1%25abc]:22000", "tcp://[fe80::1%25abc]:22000"},
	}

	for _, tc := range cases {
		u0, err := url.Parse(tc[0])
		if err != nil {
			t.Fatal(err)
		}
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

func TestGetDialer(t *testing.T) {
	mustParseURI := func(v string) *url.URL {
		uri, err := url.Parse(v)
		if err != nil {
			panic(err)
		}
		return uri
	}

	cases := []struct {
		uri        *url.URL
		ok         bool
		disabled   bool
		deprecated bool
	}{
		{mustParseURI("tcp://1.2.3.4:5678"), true, false, false},   // ok
		{mustParseURI("tcp4://1.2.3.4:5678"), true, false, false},  // ok
		{mustParseURI("kcp://1.2.3.4:5678"), false, false, true},   // deprecated
		{mustParseURI("relay://1.2.3.4:5678"), false, true, false}, // disabled
		{mustParseURI("http://1.2.3.4:5678"), false, false, false}, // generally bad
		{mustParseURI("bananas!"), false, false, false},            // wat
	}

	cfg := config.New(protocol.LocalDeviceID)
	cfg.Options.RelaysEnabled = false

	for _, tc := range cases {
		df, err := getDialerFactory(cfg, tc.uri)
		if tc.ok && err != nil {
			t.Errorf("getDialerFactory(%q) => %v, expected nil err", tc.uri, err)
		}
		if tc.ok && df == nil {
			t.Errorf("getDialerFactory(%q) => nil factory, expected non-nil", tc.uri)
		}
		if tc.deprecated && err != errDeprecated {
			t.Errorf("getDialerFactory(%q) => %v, expected %v", tc.uri, err, errDeprecated)
		}
		if tc.disabled && err != errDisabled {
			t.Errorf("getDialerFactory(%q) => %v, expected %v", tc.uri, err, errDisabled)
		}

		lf, err := getListenerFactory(cfg, tc.uri)
		if tc.ok && err != nil {
			t.Errorf("getListenerFactory(%q) => %v, expected nil err", tc.uri, err)
		}
		if tc.ok && lf == nil {
			t.Errorf("getListenerFactory(%q) => nil factory, expected non-nil", tc.uri)
		}
		if tc.deprecated && err != errDeprecated {
			t.Errorf("getListenerFactory(%q) => %v, expected %v", tc.uri, err, errDeprecated)
		}
		if tc.disabled && err != errDisabled {
			t.Errorf("getListenerFactory(%q) => %v, expected %v", tc.uri, err, errDisabled)
		}
	}
}

func TestConnectionStatus(t *testing.T) {
	s := newConnectionStatusHandler()

	addr := "testAddr"
	testErr := errors.New("testErr")

	if stats := s.ConnectionStatus(); len(stats) != 0 {
		t.Fatal("newly created connectionStatusHandler isn't empty:", len(stats))
	}

	check := func(in, out error) {
		t.Helper()
		s.setConnectionStatus(addr, in)
		switch stat, ok := s.ConnectionStatus()[addr]; {
		case !ok:
			t.Fatal("entry missing")
		case out == nil:
			if stat.Error != nil {
				t.Fatal("expected nil error, got", stat.Error)
			}
		case *stat.Error != out.Error():
			t.Fatalf("expected %v error, got %v", out.Error(), *stat.Error)
		}
	}

	check(nil, nil)

	check(context.Canceled, nil)

	check(testErr, testErr)

	check(context.Canceled, testErr)

	check(nil, nil)
}

// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"testing"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestIsLANHost(t *testing.T) {
	cases := []struct {
		addr string
		lan  bool
	}{
		// loopback
		{"127.0.0.1:22000", true},
		{"127.0.0.1", true},
		// local nets
		{"10.20.30.40:22000", true},
		{"10.20.30.40", true},
		// neither
		{"192.0.2.1:22000", false},
		{"192.0.2.1", false},
		// doesn't resolve
		{"[banana::phone]:hello", false},
		{"„‹›ﬂ´ﬁÎ‡‰ˇ¨Á˝", false},
	}

	cfg := config.Wrap("/dev/null", config.Configuration{
		Options: config.OptionsConfiguration{
			AlwaysLocalNets: []string{"10.20.30.0/24"},
		},
	}, protocol.LocalDeviceID, events.NoopLogger)
	s := &lanChecker{cfg: cfg}

	for _, tc := range cases {
		res := s.isLANHost(tc.addr)
		if res != tc.lan {
			t.Errorf("isLANHost(%q) => %v, expected %v", tc.addr, res, tc.lan)
		}
	}
}

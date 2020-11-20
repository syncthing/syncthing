// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package nat

import (
	"io/ioutil"
	"net"
	"os"
	"testing"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestMappingValidGateway(t *testing.T) {
	a := net.ParseIP("10.0.0.1")
	b := net.ParseIP("192.168.0.1")
	tests := []struct {
		mappingLocalIP net.IP
		gatewayLocalIP net.IP
		expected       bool
	}{
		// Any of the IPs is nil or unspecified implies correct
		{nil, nil, true},
		{net.IPv4zero, net.IPv4zero, true},
		{nil, net.IPv4zero, true},
		{net.IPv4zero, nil, true},
		{a, nil, true},
		{b, nil, true},
		{a, net.IPv4zero, true},
		{b, net.IPv4zero, true},
		{nil, a, true},
		{nil, b, true},
		{net.IPv4zero, a, true},
		{net.IPv4zero, b, true},
		// IPs are the same implies correct
		{a, a, true},
		{b, b, true},
		// IPs are specified and different, implies incorrect
		{a, b, false},
		{b, a, false},
	}

	for _, test := range tests {
		m := Mapping{
			address: Address{
				IP: test.mappingLocalIP,
			},
		}
		result := m.validGateway(test.gatewayLocalIP)
		if result != test.expected {
			t.Errorf("Incorrect: local %s gateway %s result %t expected %t", test.mappingLocalIP, test.gatewayLocalIP, result, test.expected)
		}
	}
}

func TestMappingClearAddresses(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "syncthing-testConfig-")
	if err != nil {
		t.Fatal(err)
	}
	w := config.Wrap(tmpFile.Name(), config.Configuration{}, protocol.LocalDeviceID, events.NoopLogger)
	defer os.RemoveAll(tmpFile.Name())
	tmpFile.Close()

	natSvc := NewService(protocol.EmptyDeviceID, w)
	// Mock a mapped port; avoids the need to actually map a port
	ip := net.ParseIP("192.168.0.1")
	m := natSvc.NewMapping(TCP, ip, 1024)
	m.extAddresses["test"] = Address{
		IP:   ip,
		Port: 1024,
	}
	// Now try and remove the mapped port; prior to #4829 this deadlocked
	natSvc.RemoveMapping(m)
}

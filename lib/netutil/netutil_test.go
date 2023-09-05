// Copyright (C) 2023 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package netutil

import "testing"

func TestAddress(t *testing.T) {
	tests := []struct {
		network string
		host    string
		result  string
	}{
		{"tcp", "google.com", "tcp://google.com"},
		{"foo", "google", "foo://google"},
		{"123", "456", "123://456"},
	}

	for _, test := range tests {
		result := AddressURL(test.network, test.host)
		if result != test.result {
			t.Errorf("%s != %s", result, test.result)
		}
	}
}

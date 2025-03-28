// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"testing"
)

func TestWinsConflict(t *testing.T) {
	testcases := [][2]FileInfo{
		// The first should always win over the second
		{{ModifiedS: 42}, {ModifiedS: 41}},
		{{ModifiedS: 41}, {ModifiedS: 42, Deleted: true}},
		{{Deleted: true}, {ModifiedS: 10, RawInvalid: true}},
		{{ModifiedS: 41, Version: Vector{Counters: []Counter{{ID: 42, Value: 2}, {ID: 43, Value: 1}}}}, {ModifiedS: 41, Version: Vector{Counters: []Counter{{ID: 42, Value: 1}, {ID: 43, Value: 2}}}}},
	}

	for _, tc := range testcases {
		if !tc[0].WinsConflict(tc[1]) {
			t.Errorf("%v should win over %v", tc[0], tc[1])
		}
		if tc[1].WinsConflict(tc[0]) {
			t.Errorf("%v should not win over %v", tc[1], tc[0])
		}
	}
}

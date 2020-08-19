// Copyright (C) 2015 The Protocol Authors.

package protocol

import "testing"

func TestWinsConflict(t *testing.T) {
	testcases := [][2]FileInfo{
		// The first should always win over the second
		{{ModifiedS: 42}, {ModifiedS: 41}},
		{{ModifiedS: 41}, {ModifiedS: 42, Deleted: true}},
		{{Deleted: true}, {ModifiedS: 10, RawInvalid: true}},
		{{ModifiedS: 41, Version: Vector{Counters: []Counter{{ID: 42, Value: 2}, {ID: 43, Value: 1}}}}, {ModifiedS: 41, Version: Vector{Counters: []Counter{{ID: 42, Value: 1}, {ID: 43, Value: 2}}}}},
	}

	for _, tc := range testcases {
		if !WinsConflict(tc[0], tc[1]) {
			t.Errorf("%v should win over %v", tc[0], tc[1])
		}
		if WinsConflict(tc[1], tc[0]) {
			t.Errorf("%v should not win over %v", tc[1], tc[0])
		}
	}
}

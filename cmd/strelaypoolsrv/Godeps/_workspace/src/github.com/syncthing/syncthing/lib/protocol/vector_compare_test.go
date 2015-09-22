// Copyright (C) 2015 The Protocol Authors.

package protocol

import (
	"math"
	"testing"
)

func TestCompare(t *testing.T) {
	testcases := []struct {
		a, b Vector
		r    Ordering
	}{
		// Empty vectors are identical
		{Vector{}, Vector{}, Equal},
		{Vector{}, nil, Equal},
		{nil, Vector{}, Equal},
		{nil, Vector{Counter{42, 0}}, Equal},
		{Vector{}, Vector{Counter{42, 0}}, Equal},
		{Vector{Counter{42, 0}}, nil, Equal},
		{Vector{Counter{42, 0}}, Vector{}, Equal},

		// Zero is the implied value for a missing Counter
		{
			Vector{Counter{42, 0}},
			Vector{Counter{77, 0}},
			Equal,
		},

		// Equal vectors are equal
		{
			Vector{Counter{42, 33}},
			Vector{Counter{42, 33}},
			Equal,
		},
		{
			Vector{Counter{42, 33}, Counter{77, 24}},
			Vector{Counter{42, 33}, Counter{77, 24}},
			Equal,
		},

		// These a-vectors are all greater than the b-vector
		{
			Vector{Counter{42, 1}},
			nil,
			Greater,
		},
		{
			Vector{Counter{42, 1}},
			Vector{},
			Greater,
		},
		{
			Vector{Counter{0, 1}},
			Vector{Counter{0, 0}},
			Greater,
		},
		{
			Vector{Counter{42, 1}},
			Vector{Counter{42, 0}},
			Greater,
		},
		{
			Vector{Counter{math.MaxUint64, 1}},
			Vector{Counter{math.MaxUint64, 0}},
			Greater,
		},
		{
			Vector{Counter{0, math.MaxUint64}},
			Vector{Counter{0, 0}},
			Greater,
		},
		{
			Vector{Counter{42, math.MaxUint64}},
			Vector{Counter{42, 0}},
			Greater,
		},
		{
			Vector{Counter{math.MaxUint64, math.MaxUint64}},
			Vector{Counter{math.MaxUint64, 0}},
			Greater,
		},
		{
			Vector{Counter{0, math.MaxUint64}},
			Vector{Counter{0, math.MaxUint64 - 1}},
			Greater,
		},
		{
			Vector{Counter{42, math.MaxUint64}},
			Vector{Counter{42, math.MaxUint64 - 1}},
			Greater,
		},
		{
			Vector{Counter{math.MaxUint64, math.MaxUint64}},
			Vector{Counter{math.MaxUint64, math.MaxUint64 - 1}},
			Greater,
		},
		{
			Vector{Counter{42, 2}},
			Vector{Counter{42, 1}},
			Greater,
		},
		{
			Vector{Counter{22, 22}, Counter{42, 2}},
			Vector{Counter{22, 22}, Counter{42, 1}},
			Greater,
		},
		{
			Vector{Counter{42, 2}, Counter{77, 3}},
			Vector{Counter{42, 1}, Counter{77, 3}},
			Greater,
		},
		{
			Vector{Counter{22, 22}, Counter{42, 2}, Counter{77, 3}},
			Vector{Counter{22, 22}, Counter{42, 1}, Counter{77, 3}},
			Greater,
		},
		{
			Vector{Counter{22, 23}, Counter{42, 2}, Counter{77, 4}},
			Vector{Counter{22, 22}, Counter{42, 1}, Counter{77, 3}},
			Greater,
		},

		// These a-vectors are all lesser than the b-vector
		{nil, Vector{Counter{42, 1}}, Lesser},
		{Vector{}, Vector{Counter{42, 1}}, Lesser},
		{
			Vector{Counter{42, 0}},
			Vector{Counter{42, 1}},
			Lesser,
		},
		{
			Vector{Counter{42, 1}},
			Vector{Counter{42, 2}},
			Lesser,
		},
		{
			Vector{Counter{22, 22}, Counter{42, 1}},
			Vector{Counter{22, 22}, Counter{42, 2}},
			Lesser,
		},
		{
			Vector{Counter{42, 1}, Counter{77, 3}},
			Vector{Counter{42, 2}, Counter{77, 3}},
			Lesser,
		},
		{
			Vector{Counter{22, 22}, Counter{42, 1}, Counter{77, 3}},
			Vector{Counter{22, 22}, Counter{42, 2}, Counter{77, 3}},
			Lesser,
		},
		{
			Vector{Counter{22, 22}, Counter{42, 1}, Counter{77, 3}},
			Vector{Counter{22, 23}, Counter{42, 2}, Counter{77, 4}},
			Lesser,
		},

		// These are all in conflict
		{
			Vector{Counter{42, 2}},
			Vector{Counter{43, 1}},
			ConcurrentGreater,
		},
		{
			Vector{Counter{43, 1}},
			Vector{Counter{42, 2}},
			ConcurrentLesser,
		},
		{
			Vector{Counter{22, 23}, Counter{42, 1}},
			Vector{Counter{22, 22}, Counter{42, 2}},
			ConcurrentGreater,
		},
		{
			Vector{Counter{22, 21}, Counter{42, 2}},
			Vector{Counter{22, 22}, Counter{42, 1}},
			ConcurrentLesser,
		},
		{
			Vector{Counter{22, 21}, Counter{42, 2}, Counter{43, 1}},
			Vector{Counter{20, 1}, Counter{22, 22}, Counter{42, 1}},
			ConcurrentLesser,
		},
	}

	for i, tc := range testcases {
		// Test real Compare
		if r := tc.a.Compare(tc.b); r != tc.r {
			t.Errorf("%d: %+v.Compare(%+v) == %v (expected %v)", i, tc.a, tc.b, r, tc.r)
		}

		// Test convenience functions
		switch tc.r {
		case Greater:
			if tc.a.Equal(tc.b) {
				t.Errorf("%+v == %+v", tc.a, tc.b)
			}
			if tc.a.Concurrent(tc.b) {
				t.Errorf("%+v concurrent %+v", tc.a, tc.b)
			}
			if !tc.a.GreaterEqual(tc.b) {
				t.Errorf("%+v not >= %+v", tc.a, tc.b)
			}
			if tc.a.LesserEqual(tc.b) {
				t.Errorf("%+v <= %+v", tc.a, tc.b)
			}
		case Lesser:
			if tc.a.Concurrent(tc.b) {
				t.Errorf("%+v concurrent %+v", tc.a, tc.b)
			}
			if tc.a.Equal(tc.b) {
				t.Errorf("%+v == %+v", tc.a, tc.b)
			}
			if tc.a.GreaterEqual(tc.b) {
				t.Errorf("%+v >= %+v", tc.a, tc.b)
			}
			if !tc.a.LesserEqual(tc.b) {
				t.Errorf("%+v not <= %+v", tc.a, tc.b)
			}
		case Equal:
			if tc.a.Concurrent(tc.b) {
				t.Errorf("%+v concurrent %+v", tc.a, tc.b)
			}
			if !tc.a.Equal(tc.b) {
				t.Errorf("%+v not == %+v", tc.a, tc.b)
			}
			if !tc.a.GreaterEqual(tc.b) {
				t.Errorf("%+v not <= %+v", tc.a, tc.b)
			}
			if !tc.a.LesserEqual(tc.b) {
				t.Errorf("%+v not <= %+v", tc.a, tc.b)
			}
		case ConcurrentLesser, ConcurrentGreater:
			if !tc.a.Concurrent(tc.b) {
				t.Errorf("%+v not concurrent %+v", tc.a, tc.b)
			}
			if tc.a.Equal(tc.b) {
				t.Errorf("%+v == %+v", tc.a, tc.b)
			}
			if tc.a.GreaterEqual(tc.b) {
				t.Errorf("%+v >= %+v", tc.a, tc.b)
			}
			if tc.a.LesserEqual(tc.b) {
				t.Errorf("%+v <= %+v", tc.a, tc.b)
			}
		}
	}
}

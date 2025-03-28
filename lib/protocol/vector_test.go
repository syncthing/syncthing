// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"math"
	"testing"
)

func TestUpdate(t *testing.T) {
	var v Vector

	// Append

	v = v.updateWithNow(42, 5)
	expected := Vector{Counters: []Counter{{ID: 42, Value: 5}}}

	if v.Compare(expected) != Equal {
		t.Errorf("Update error, %+v != %+v", v, expected)
	}

	// Insert at front

	v = v.updateWithNow(36, 6)
	expected = Vector{Counters: []Counter{{ID: 36, Value: 6}, {ID: 42, Value: 5}}}

	if v.Compare(expected) != Equal {
		t.Errorf("Update error, %+v != %+v", v, expected)
	}

	// Insert in middle

	v = v.updateWithNow(37, 7)
	expected = Vector{Counters: []Counter{{ID: 36, Value: 6}, {ID: 37, Value: 7}, {ID: 42, Value: 5}}}

	if v.Compare(expected) != Equal {
		t.Errorf("Update error, %+v != %+v", v, expected)
	}

	// Update existing

	v = v.updateWithNow(37, 1)
	expected = Vector{Counters: []Counter{{ID: 36, Value: 6}, {ID: 37, Value: 8}, {ID: 42, Value: 5}}}

	if v.Compare(expected) != Equal {
		t.Errorf("Update error, %+v != %+v", v, expected)
	}

	// Update existing with higher current time

	v = v.updateWithNow(37, 100)
	expected = Vector{Counters: []Counter{{ID: 36, Value: 6}, {ID: 37, Value: 100}, {ID: 42, Value: 5}}}

	if v.Compare(expected) != Equal {
		t.Errorf("Update error, %+v != %+v", v, expected)
	}

	// Update existing with lower current time

	v = v.updateWithNow(37, 50)
	expected = Vector{Counters: []Counter{{ID: 36, Value: 6}, {ID: 37, Value: 101}, {ID: 42, Value: 5}}}

	if v.Compare(expected) != Equal {
		t.Errorf("Update error, %+v != %+v", v, expected)
	}
}

func TestCopy(t *testing.T) {
	v0 := Vector{Counters: []Counter{{ID: 42, Value: 1}}}
	v1 := v0.Copy()
	v1.Update(42)
	if v0.Compare(v1) != Lesser {
		t.Errorf("Copy error, %+v should be ancestor of %+v", v0, v1)
	}
}

func TestMerge(t *testing.T) {
	testcases := []struct {
		a, b, m Vector
	}{
		// No-ops
		{
			Vector{},
			Vector{},
			Vector{},
		},
		{
			Vector{Counters: []Counter{{ID: 22, Value: 1}, {ID: 42, Value: 1}}},
			Vector{Counters: []Counter{{ID: 22, Value: 1}, {ID: 42, Value: 1}}},
			Vector{Counters: []Counter{{ID: 22, Value: 1}, {ID: 42, Value: 1}}},
		},

		// Appends
		{
			Vector{},
			Vector{Counters: []Counter{{ID: 22, Value: 1}, {ID: 42, Value: 1}}},
			Vector{Counters: []Counter{{ID: 22, Value: 1}, {ID: 42, Value: 1}}},
		},
		{
			Vector{Counters: []Counter{{ID: 22, Value: 1}}},
			Vector{Counters: []Counter{{ID: 42, Value: 1}}},
			Vector{Counters: []Counter{{ID: 22, Value: 1}, {ID: 42, Value: 1}}},
		},
		{
			Vector{Counters: []Counter{{ID: 22, Value: 1}}},
			Vector{Counters: []Counter{{ID: 22, Value: 1}, {ID: 42, Value: 1}}},
			Vector{Counters: []Counter{{ID: 22, Value: 1}, {ID: 42, Value: 1}}},
		},

		// Insert
		{
			Vector{Counters: []Counter{{ID: 22, Value: 1}, {ID: 42, Value: 1}}},
			Vector{Counters: []Counter{{ID: 22, Value: 1}, {ID: 23, Value: 2}, {ID: 42, Value: 1}}},
			Vector{Counters: []Counter{{ID: 22, Value: 1}, {ID: 23, Value: 2}, {ID: 42, Value: 1}}},
		},
		{
			Vector{Counters: []Counter{{ID: 42, Value: 1}}},
			Vector{Counters: []Counter{{ID: 22, Value: 1}}},
			Vector{Counters: []Counter{{ID: 22, Value: 1}, {ID: 42, Value: 1}}},
		},

		// Update
		{
			Vector{Counters: []Counter{{ID: 22, Value: 1}, {ID: 42, Value: 2}}},
			Vector{Counters: []Counter{{ID: 22, Value: 2}, {ID: 42, Value: 1}}},
			Vector{Counters: []Counter{{ID: 22, Value: 2}, {ID: 42, Value: 2}}},
		},

		// All of the above
		{
			Vector{Counters: []Counter{{ID: 10, Value: 1}, {ID: 20, Value: 2}, {ID: 30, Value: 1}}},
			Vector{Counters: []Counter{{ID: 5, Value: 1}, {ID: 10, Value: 2}, {ID: 15, Value: 1}, {ID: 20, Value: 1}, {ID: 25, Value: 1}, {ID: 35, Value: 1}}},
			Vector{Counters: []Counter{{ID: 5, Value: 1}, {ID: 10, Value: 2}, {ID: 15, Value: 1}, {ID: 20, Value: 2}, {ID: 25, Value: 1}, {ID: 30, Value: 1}, {ID: 35, Value: 1}}},
		},
	}

	for i, tc := range testcases {
		if m := tc.a.Merge(tc.b); m.Compare(tc.m) != Equal {
			t.Errorf("%d: %+v.Merge(%+v) == %+v (expected %+v)", i, tc.a, tc.b, m, tc.m)
		}
	}
}

func TestCounterValue(t *testing.T) {
	v0 := Vector{Counters: []Counter{{ID: 42, Value: 1}, {ID: 64, Value: 5}}}
	if v0.Counter(42) != 1 {
		t.Errorf("Counter error, %d != %d", v0.Counter(42), 1)
	}
	if v0.Counter(64) != 5 {
		t.Errorf("Counter error, %d != %d", v0.Counter(64), 5)
	}
	if v0.Counter(72) != 0 {
		t.Errorf("Counter error, %d != %d", v0.Counter(72), 0)
	}
}

func TestCompare(t *testing.T) {
	testcases := []struct {
		a, b Vector
		r    Ordering
	}{
		// Empty vectors are identical
		{Vector{}, Vector{}, Equal},
		{Vector{}, Vector{Counters: []Counter{{ID: 42, Value: 0}}}, Equal},
		{Vector{Counters: []Counter{{ID: 42, Value: 0}}}, Vector{}, Equal},

		// Zero is the implied value for a missing Counter
		{
			Vector{Counters: []Counter{{ID: 42, Value: 0}}},
			Vector{Counters: []Counter{{ID: 77, Value: 0}}},
			Equal,
		},

		// Equal vectors are equal
		{
			Vector{Counters: []Counter{{ID: 42, Value: 33}}},
			Vector{Counters: []Counter{{ID: 42, Value: 33}}},
			Equal,
		},
		{
			Vector{Counters: []Counter{{ID: 42, Value: 33}, {ID: 77, Value: 24}}},
			Vector{Counters: []Counter{{ID: 42, Value: 33}, {ID: 77, Value: 24}}},
			Equal,
		},

		// These a-vectors are all greater than the b-vector
		{
			Vector{Counters: []Counter{{ID: 42, Value: 1}}},
			Vector{},
			Greater,
		},
		{
			Vector{Counters: []Counter{{ID: 0, Value: 1}}},
			Vector{Counters: []Counter{{ID: 0, Value: 0}}},
			Greater,
		},
		{
			Vector{Counters: []Counter{{ID: 42, Value: 1}}},
			Vector{Counters: []Counter{{ID: 42, Value: 0}}},
			Greater,
		},
		{
			Vector{Counters: []Counter{{ID: math.MaxUint64, Value: 1}}},
			Vector{Counters: []Counter{{ID: math.MaxUint64, Value: 0}}},
			Greater,
		},
		{
			Vector{Counters: []Counter{{ID: 0, Value: math.MaxUint64}}},
			Vector{Counters: []Counter{{ID: 0, Value: 0}}},
			Greater,
		},
		{
			Vector{Counters: []Counter{{ID: 42, Value: math.MaxUint64}}},
			Vector{Counters: []Counter{{ID: 42, Value: 0}}},
			Greater,
		},
		{
			Vector{Counters: []Counter{{ID: math.MaxUint64, Value: math.MaxUint64}}},
			Vector{Counters: []Counter{{ID: math.MaxUint64, Value: 0}}},
			Greater,
		},
		{
			Vector{Counters: []Counter{{ID: 0, Value: math.MaxUint64}}},
			Vector{Counters: []Counter{{ID: 0, Value: math.MaxUint64 - 1}}},
			Greater,
		},
		{
			Vector{Counters: []Counter{{ID: 42, Value: math.MaxUint64}}},
			Vector{Counters: []Counter{{ID: 42, Value: math.MaxUint64 - 1}}},
			Greater,
		},
		{
			Vector{Counters: []Counter{{ID: math.MaxUint64, Value: math.MaxUint64}}},
			Vector{Counters: []Counter{{ID: math.MaxUint64, Value: math.MaxUint64 - 1}}},
			Greater,
		},
		{
			Vector{Counters: []Counter{{ID: 42, Value: 2}}},
			Vector{Counters: []Counter{{ID: 42, Value: 1}}},
			Greater,
		},
		{
			Vector{Counters: []Counter{{ID: 22, Value: 22}, {ID: 42, Value: 2}}},
			Vector{Counters: []Counter{{ID: 22, Value: 22}, {ID: 42, Value: 1}}},
			Greater,
		},
		{
			Vector{Counters: []Counter{{ID: 42, Value: 2}, {ID: 77, Value: 3}}},
			Vector{Counters: []Counter{{ID: 42, Value: 1}, {ID: 77, Value: 3}}},
			Greater,
		},
		{
			Vector{Counters: []Counter{{ID: 22, Value: 22}, {ID: 42, Value: 2}, {ID: 77, Value: 3}}},
			Vector{Counters: []Counter{{ID: 22, Value: 22}, {ID: 42, Value: 1}, {ID: 77, Value: 3}}},
			Greater,
		},
		{
			Vector{Counters: []Counter{{ID: 22, Value: 23}, {ID: 42, Value: 2}, {ID: 77, Value: 4}}},
			Vector{Counters: []Counter{{ID: 22, Value: 22}, {ID: 42, Value: 1}, {ID: 77, Value: 3}}},
			Greater,
		},

		// These a-vectors are all lesser than the b-vector
		{Vector{}, Vector{Counters: []Counter{{ID: 42, Value: 1}}}, Lesser},
		{
			Vector{Counters: []Counter{{ID: 42, Value: 0}}},
			Vector{Counters: []Counter{{ID: 42, Value: 1}}},
			Lesser,
		},
		{
			Vector{Counters: []Counter{{ID: 42, Value: 1}}},
			Vector{Counters: []Counter{{ID: 42, Value: 2}}},
			Lesser,
		},
		{
			Vector{Counters: []Counter{{ID: 22, Value: 22}, {ID: 42, Value: 1}}},
			Vector{Counters: []Counter{{ID: 22, Value: 22}, {ID: 42, Value: 2}}},
			Lesser,
		},
		{
			Vector{Counters: []Counter{{ID: 42, Value: 1}, {ID: 77, Value: 3}}},
			Vector{Counters: []Counter{{ID: 42, Value: 2}, {ID: 77, Value: 3}}},
			Lesser,
		},
		{
			Vector{Counters: []Counter{{ID: 22, Value: 22}, {ID: 42, Value: 1}, {ID: 77, Value: 3}}},
			Vector{Counters: []Counter{{ID: 22, Value: 22}, {ID: 42, Value: 2}, {ID: 77, Value: 3}}},
			Lesser,
		},
		{
			Vector{Counters: []Counter{{ID: 22, Value: 22}, {ID: 42, Value: 1}, {ID: 77, Value: 3}}},
			Vector{Counters: []Counter{{ID: 22, Value: 23}, {ID: 42, Value: 2}, {ID: 77, Value: 4}}},
			Lesser,
		},

		// These are all in conflict
		{
			Vector{Counters: []Counter{{ID: 42, Value: 2}}},
			Vector{Counters: []Counter{{ID: 43, Value: 1}}},
			ConcurrentGreater,
		},
		{
			Vector{Counters: []Counter{{ID: 43, Value: 1}}},
			Vector{Counters: []Counter{{ID: 42, Value: 2}}},
			ConcurrentLesser,
		},
		{
			Vector{Counters: []Counter{{ID: 22, Value: 23}, {ID: 42, Value: 1}}},
			Vector{Counters: []Counter{{ID: 22, Value: 22}, {ID: 42, Value: 2}}},
			ConcurrentGreater,
		},
		{
			Vector{Counters: []Counter{{ID: 22, Value: 21}, {ID: 42, Value: 2}}},
			Vector{Counters: []Counter{{ID: 22, Value: 22}, {ID: 42, Value: 1}}},
			ConcurrentLesser,
		},
		{
			Vector{Counters: []Counter{{ID: 22, Value: 21}, {ID: 42, Value: 2}, {ID: 43, Value: 1}}},
			Vector{Counters: []Counter{{ID: 20, Value: 1}, {ID: 22, Value: 22}, {ID: 42, Value: 1}}},
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

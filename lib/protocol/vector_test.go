// Copyright (C) 2015 The Protocol Authors.

package protocol

import (
	"math"
	"testing"
)

func TestUpdate(t *testing.T) {
	var v Vector

	// Append

	v = v.Update(42)
	expected := Vector{[]Counter{{42, 1}}}

	if v.Compare(expected) != Equal {
		t.Errorf("Update error, %+v != %+v", v, expected)
	}

	// Insert at front

	v = v.Update(36)
	expected = Vector{[]Counter{{36, 1}, {42, 1}}}

	if v.Compare(expected) != Equal {
		t.Errorf("Update error, %+v != %+v", v, expected)
	}

	// Insert in moddle

	v = v.Update(37)
	expected = Vector{[]Counter{{36, 1}, {37, 1}, {42, 1}}}

	if v.Compare(expected) != Equal {
		t.Errorf("Update error, %+v != %+v", v, expected)
	}

	// Update existing

	v = v.Update(37)
	expected = Vector{[]Counter{{36, 1}, {37, 2}, {42, 1}}}

	if v.Compare(expected) != Equal {
		t.Errorf("Update error, %+v != %+v", v, expected)
	}
}

func TestCopy(t *testing.T) {
	v0 := Vector{[]Counter{{42, 1}}}
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
			Vector{[]Counter{{22, 1}, {42, 1}}},
			Vector{[]Counter{{22, 1}, {42, 1}}},
			Vector{[]Counter{{22, 1}, {42, 1}}},
		},

		// Appends
		{
			Vector{},
			Vector{[]Counter{{22, 1}, {42, 1}}},
			Vector{[]Counter{{22, 1}, {42, 1}}},
		},
		{
			Vector{[]Counter{{22, 1}}},
			Vector{[]Counter{{42, 1}}},
			Vector{[]Counter{{22, 1}, {42, 1}}},
		},
		{
			Vector{[]Counter{{22, 1}}},
			Vector{[]Counter{{22, 1}, {42, 1}}},
			Vector{[]Counter{{22, 1}, {42, 1}}},
		},

		// Insert
		{
			Vector{[]Counter{{22, 1}, {42, 1}}},
			Vector{[]Counter{{22, 1}, {23, 2}, {42, 1}}},
			Vector{[]Counter{{22, 1}, {23, 2}, {42, 1}}},
		},
		{
			Vector{[]Counter{{42, 1}}},
			Vector{[]Counter{{22, 1}}},
			Vector{[]Counter{{22, 1}, {42, 1}}},
		},

		// Update
		{
			Vector{[]Counter{{22, 1}, {42, 2}}},
			Vector{[]Counter{{22, 2}, {42, 1}}},
			Vector{[]Counter{{22, 2}, {42, 2}}},
		},

		// All of the above
		{
			Vector{[]Counter{{10, 1}, {20, 2}, {30, 1}}},
			Vector{[]Counter{{5, 1}, {10, 2}, {15, 1}, {20, 1}, {25, 1}, {35, 1}}},
			Vector{[]Counter{{5, 1}, {10, 2}, {15, 1}, {20, 2}, {25, 1}, {30, 1}, {35, 1}}},
		},
	}

	for i, tc := range testcases {
		if m := tc.a.Merge(tc.b); m.Compare(tc.m) != Equal {
			t.Errorf("%d: %+v.Merge(%+v) == %+v (expected %+v)", i, tc.a, tc.b, m, tc.m)
		}
	}
}

func TestCounterValue(t *testing.T) {
	v0 := Vector{[]Counter{{42, 1}, {64, 5}}}
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
		{Vector{}, Vector{[]Counter{{42, 0}}}, Equal},
		{Vector{[]Counter{{42, 0}}}, Vector{}, Equal},

		// Zero is the implied value for a missing Counter
		{
			Vector{[]Counter{{42, 0}}},
			Vector{[]Counter{{77, 0}}},
			Equal,
		},

		// Equal vectors are equal
		{
			Vector{[]Counter{{42, 33}}},
			Vector{[]Counter{{42, 33}}},
			Equal,
		},
		{
			Vector{[]Counter{{42, 33}, {77, 24}}},
			Vector{[]Counter{{42, 33}, {77, 24}}},
			Equal,
		},

		// These a-vectors are all greater than the b-vector
		{
			Vector{[]Counter{{42, 1}}},
			Vector{},
			Greater,
		},
		{
			Vector{[]Counter{{0, 1}}},
			Vector{[]Counter{{0, 0}}},
			Greater,
		},
		{
			Vector{[]Counter{{42, 1}}},
			Vector{[]Counter{{42, 0}}},
			Greater,
		},
		{
			Vector{[]Counter{{math.MaxUint64, 1}}},
			Vector{[]Counter{{math.MaxUint64, 0}}},
			Greater,
		},
		{
			Vector{[]Counter{{0, math.MaxUint64}}},
			Vector{[]Counter{{0, 0}}},
			Greater,
		},
		{
			Vector{[]Counter{{42, math.MaxUint64}}},
			Vector{[]Counter{{42, 0}}},
			Greater,
		},
		{
			Vector{[]Counter{{math.MaxUint64, math.MaxUint64}}},
			Vector{[]Counter{{math.MaxUint64, 0}}},
			Greater,
		},
		{
			Vector{[]Counter{{0, math.MaxUint64}}},
			Vector{[]Counter{{0, math.MaxUint64 - 1}}},
			Greater,
		},
		{
			Vector{[]Counter{{42, math.MaxUint64}}},
			Vector{[]Counter{{42, math.MaxUint64 - 1}}},
			Greater,
		},
		{
			Vector{[]Counter{{math.MaxUint64, math.MaxUint64}}},
			Vector{[]Counter{{math.MaxUint64, math.MaxUint64 - 1}}},
			Greater,
		},
		{
			Vector{[]Counter{{42, 2}}},
			Vector{[]Counter{{42, 1}}},
			Greater,
		},
		{
			Vector{[]Counter{{22, 22}, {42, 2}}},
			Vector{[]Counter{{22, 22}, {42, 1}}},
			Greater,
		},
		{
			Vector{[]Counter{{42, 2}, {77, 3}}},
			Vector{[]Counter{{42, 1}, {77, 3}}},
			Greater,
		},
		{
			Vector{[]Counter{{22, 22}, {42, 2}, {77, 3}}},
			Vector{[]Counter{{22, 22}, {42, 1}, {77, 3}}},
			Greater,
		},
		{
			Vector{[]Counter{{22, 23}, {42, 2}, {77, 4}}},
			Vector{[]Counter{{22, 22}, {42, 1}, {77, 3}}},
			Greater,
		},

		// These a-vectors are all lesser than the b-vector
		{Vector{}, Vector{[]Counter{{42, 1}}}, Lesser},
		{
			Vector{[]Counter{{42, 0}}},
			Vector{[]Counter{{42, 1}}},
			Lesser,
		},
		{
			Vector{[]Counter{{42, 1}}},
			Vector{[]Counter{{42, 2}}},
			Lesser,
		},
		{
			Vector{[]Counter{{22, 22}, {42, 1}}},
			Vector{[]Counter{{22, 22}, {42, 2}}},
			Lesser,
		},
		{
			Vector{[]Counter{{42, 1}, {77, 3}}},
			Vector{[]Counter{{42, 2}, {77, 3}}},
			Lesser,
		},
		{
			Vector{[]Counter{{22, 22}, {42, 1}, {77, 3}}},
			Vector{[]Counter{{22, 22}, {42, 2}, {77, 3}}},
			Lesser,
		},
		{
			Vector{[]Counter{{22, 22}, {42, 1}, {77, 3}}},
			Vector{[]Counter{{22, 23}, {42, 2}, {77, 4}}},
			Lesser,
		},

		// These are all in conflict
		{
			Vector{[]Counter{{42, 2}}},
			Vector{[]Counter{{43, 1}}},
			ConcurrentGreater,
		},
		{
			Vector{[]Counter{{43, 1}}},
			Vector{[]Counter{{42, 2}}},
			ConcurrentLesser,
		},
		{
			Vector{[]Counter{{22, 23}, {42, 1}}},
			Vector{[]Counter{{22, 22}, {42, 2}}},
			ConcurrentGreater,
		},
		{
			Vector{[]Counter{{22, 21}, {42, 2}}},
			Vector{[]Counter{{22, 22}, {42, 1}}},
			ConcurrentLesser,
		},
		{
			Vector{[]Counter{{22, 21}, {42, 2}, {43, 1}}},
			Vector{[]Counter{{20, 1}, {22, 22}, {42, 1}}},
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

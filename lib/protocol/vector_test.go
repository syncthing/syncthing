// Copyright (C) 2015 The Protocol Authors.

package protocol

import "testing"

func TestUpdate(t *testing.T) {
	var v Vector

	// Append

	v = v.Update(42)
	expected := Vector{Counter{42, 1}}

	if v.Compare(expected) != Equal {
		t.Errorf("Update error, %+v != %+v", v, expected)
	}

	// Insert at front

	v = v.Update(36)
	expected = Vector{Counter{36, 1}, Counter{42, 1}}

	if v.Compare(expected) != Equal {
		t.Errorf("Update error, %+v != %+v", v, expected)
	}

	// Insert in moddle

	v = v.Update(37)
	expected = Vector{Counter{36, 1}, Counter{37, 1}, Counter{42, 1}}

	if v.Compare(expected) != Equal {
		t.Errorf("Update error, %+v != %+v", v, expected)
	}

	// Update existing

	v = v.Update(37)
	expected = Vector{Counter{36, 1}, Counter{37, 2}, Counter{42, 1}}

	if v.Compare(expected) != Equal {
		t.Errorf("Update error, %+v != %+v", v, expected)
	}
}

func TestCopy(t *testing.T) {
	v0 := Vector{Counter{42, 1}}
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
			Vector{Counter{22, 1}, Counter{42, 1}},
			Vector{Counter{22, 1}, Counter{42, 1}},
			Vector{Counter{22, 1}, Counter{42, 1}},
		},

		// Appends
		{
			Vector{},
			Vector{Counter{22, 1}, Counter{42, 1}},
			Vector{Counter{22, 1}, Counter{42, 1}},
		},
		{
			Vector{Counter{22, 1}},
			Vector{Counter{42, 1}},
			Vector{Counter{22, 1}, Counter{42, 1}},
		},
		{
			Vector{Counter{22, 1}},
			Vector{Counter{22, 1}, Counter{42, 1}},
			Vector{Counter{22, 1}, Counter{42, 1}},
		},

		// Insert
		{
			Vector{Counter{22, 1}, Counter{42, 1}},
			Vector{Counter{22, 1}, Counter{23, 2}, Counter{42, 1}},
			Vector{Counter{22, 1}, Counter{23, 2}, Counter{42, 1}},
		},
		{
			Vector{Counter{42, 1}},
			Vector{Counter{22, 1}},
			Vector{Counter{22, 1}, Counter{42, 1}},
		},

		// Update
		{
			Vector{Counter{22, 1}, Counter{42, 2}},
			Vector{Counter{22, 2}, Counter{42, 1}},
			Vector{Counter{22, 2}, Counter{42, 2}},
		},

		// All of the above
		{
			Vector{Counter{10, 1}, Counter{20, 2}, Counter{30, 1}},
			Vector{Counter{5, 1}, Counter{10, 2}, Counter{15, 1}, Counter{20, 1}, Counter{25, 1}, Counter{35, 1}},
			Vector{Counter{5, 1}, Counter{10, 2}, Counter{15, 1}, Counter{20, 2}, Counter{25, 1}, Counter{30, 1}, Counter{35, 1}},
		},
	}

	for i, tc := range testcases {
		if m := tc.a.Merge(tc.b); m.Compare(tc.m) != Equal {
			t.Errorf("%d: %+v.Merge(%+v) == %+v (expected %+v)", i, tc.a, tc.b, m, tc.m)
		}
	}
}

func TestCounterValue(t *testing.T) {
	v0 := Vector{Counter{42, 1}, Counter{64, 5}}
	if v0.Counter(42) != 1 {
		t.Error("Counter error, %d != %d", v0.Counter(42), 1)
	}
	if v0.Counter(64) != 5 {
		t.Error("Counter error, %d != %d", v0.Counter(64), 5)
	}
	if v0.Counter(72) != 0 {
		t.Error("Counter error, %d != %d", v0.Counter(72), 0)
	}
}

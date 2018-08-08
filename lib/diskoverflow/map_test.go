// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package diskoverflow

import (
	"testing"
)

func TestMapReal(t *testing.T) {
	testMap(t)
}

func TestMapNoMem(t *testing.T) {
	withAdjustedMem(t, int64(0), testMap)
}

func TestMap100B(t *testing.T) {
	withAdjustedMem(t, int64(100), testMap)
}

func TestMap100kB(t *testing.T) {
	withAdjustedMem(t, int64(100000), testMap)
}

func testMap(t *testing.T) {
	Map := NewMap(".", &testValue{})
	defer Map.Close()

	testValueSlice := randomTestValues(testSize)
	testValues := make(map[string]Value, len(testValueSlice))
	for _, v := range testValueSlice {
		testValues[v.(*testValue).string] = v
	}

	for i, tv := range testValueSlice {
		if i%100 == 0 {
			if l := Map.Length(); l != i {
				t.Errorf("s.Length() == %v, expected %v", l, i)
			}
		}
		Map.Add(tv.(*testValue).string, tv)
	}

	gotValues := make(map[string]struct{}, len(testValueSlice))
	Map.Iter(func(k string, v Value) bool {
		got := v.(*testValue).string
		if _, ok := gotValues[k]; ok {
			t.Errorf("Iterating; got %v more than once", k)
			return false
		}
		if k != got {
			t.Errorf("Iterating; key, value: %v != %v", k, got)
			return false
		}
		if _, ok := testValues[k]; !ok {
			t.Errorf("Iterating; got unexpected %v", k)
			return false
		}
		gotValues[k] = struct{}{}
		return true
	})
	if len(gotValues) != len(testValues) {
		t.Errorf("Received just %v files, expected %v", len(gotValues), len(testValues))
	}

	if l := Map.Length(); l != len(testValues) {
		t.Errorf("s.Length() == %v, expected %v", l, len(testValues))
	}

	k := len(testValues) / 2
	exp := testValueSlice[k].(*testValue).string

	v, ok := Map.Get(exp)
	if !ok {
		t.Fatalf("PopFirst didn't return any value")
	}
	if got := v.(*testValue).string; got != exp {
		t.Errorf("PopFirst: %v != %v", got, exp)
	}
	if l := Map.Length(); l != len(testValues) {
		t.Errorf("s.Length() == %v, expected %v", l, len(testValues))
	}

	v, ok = Map.Pop(exp)
	if !ok {
		t.Fatalf("PopLast didn't return any value")
	}
	if got := v.(*testValue).string; got != exp {
		t.Errorf("PopLast: %v != %v", got, exp)
	}
	if l := Map.Length(); l != len(testValues)-1 {
		t.Errorf("s.Length() == %v, expected %v", l, len(testValues)-1)
	}
	testValueSlice = append(testValueSlice[:k], testValueSlice[k+1:]...)
	delete(testValues, exp)

	gotValues = make(map[string]struct{}, len(testValueSlice))
	Map.IterAndClose(func(k string, v Value) bool {
		got := v.(*testValue).string
		if _, ok := gotValues[k]; ok {
			t.Errorf("Iterating; got %v more than once", k)
			return false
		}
		if k != got {
			t.Errorf("Iterating; key, value: %v != %v", k, got)
			return false
		}
		if _, ok := testValues[k]; !ok {
			t.Errorf("Iterating; got unexpected %v", k)
			return false
		}
		gotValues[k] = struct{}{}
		return true
	})
	if len(gotValues) != len(testValues) {
		t.Errorf("Received just %v files, expected %v", len(gotValues), len(testValues))
	}
}

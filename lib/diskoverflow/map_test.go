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
	withAdjustedMem(t, 0, testMap)
}

func TestMap100B(t *testing.T) {
	withAdjustedMem(t, 100, testMap)
}

func TestMap100kB(t *testing.T) {
	withAdjustedMem(t, 100000, testMap)
}

func testMap(t *testing.T) {
	Map := NewMap(".")
	defer Map.Close()

	testValueSlice := randomTestValues(testSize)
	testValues := make(map[string]Value, len(testValueSlice))
	for _, v := range testValueSlice {
		testValues[v.string] = v
	}

	for i, tv := range testValueSlice {
		if i%100 == 0 {
			if l := Map.Items(); l != i {
				t.Fatalf("s.Items() == %v, expected %v", l, i)
			}
		}
		Map.Set(tv.string, tv)
	}

	gotValues := make(map[string]struct{}, len(testValueSlice))
	it := Map.NewIterator()
	v := &testValue{}
	for it.Next() {
		k := it.Key()
		if _, ok := gotValues[k]; ok {
			t.Fatalf("Iterating; got %v more than once", k)
		}
		v.Reset()
		it.Value(v)
		if k != v.string {
			t.Fatalf("Iterating; key, value: %v != %v", k, v.string)
		}
		if _, ok := testValues[k]; !ok {
			t.Fatalf("Iterating; got unexpected %v", k)
		}
		gotValues[k] = struct{}{}
	}
	it.Release()
	if len(gotValues) != len(testValues) {
		t.Fatalf("Received just %v files, expected %v", len(gotValues), len(testValues))
	}

	if l := Map.Items(); l != len(testValues) {
		t.Fatalf("s.Items() == %v, expected %v", l, len(testValues))
	}

	k := len(testValues) / 2
	exp := testValueSlice[k].string

	v.Reset()
	ok := Map.Get(exp, v)
	if !ok {
		t.Fatalf("Get didn't return any value")
	}
	if got := v.string; got != exp {
		t.Fatalf("Get: %v != %v", got, exp)
	}
	if l := Map.Items(); l != len(testValues) {
		t.Fatalf("s.Items() == %v, expected %v", l, len(testValues))
	}

	v.Reset()
	ok = Map.Pop(exp, v)
	if !ok {
		t.Fatalf("Pop didn't return any value")
	}
	if got := v.string; got != exp {
		t.Fatalf("Pop %v != %v", got, exp)
	}
	if l := Map.Items(); l != len(testValues)-1 {
		t.Fatalf("s.Items() == %v, expected %v", l, len(testValues)-1)
	}
	testValueSlice = append(testValueSlice[:k], testValueSlice[k+1:]...)
	delete(testValues, exp)

	gotValues = make(map[string]struct{}, len(testValueSlice))
	it = Map.NewIterator()
	for it.Next() {
		k := it.Key()
		if _, ok := gotValues[k]; ok {
			t.Fatalf("Iterating; got %v more than once", k)
		}
		it.Value(v)
		if k != v.string {
			t.Fatalf("Iterating; key, value: %v != %v", k, v.string)
		}
		if _, ok := testValues[k]; !ok {
			t.Fatalf("Iterating; got unexpected %v", k)
		}
		gotValues[k] = struct{}{}
	}
	it.Release()
	if len(gotValues) != len(testValues) {
		t.Fatalf("Received just %v files, expected %v", len(gotValues), len(testValues))
	}
}

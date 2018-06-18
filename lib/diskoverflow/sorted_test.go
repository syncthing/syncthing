// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package diskoverflow

import (
	"sort"
	"testing"
)

func TestSortedReal(t *testing.T) {
	testSorted(t)
}

func TestSortedNoMem(t *testing.T) {
	withAdjustedMem(t, int64(0), testSorted)
}

func TestSorted100B(t *testing.T) {
	withAdjustedMem(t, int64(100), testSorted)
}

func TestSorted100kB(t *testing.T) {
	withAdjustedMem(t, int64(100000), testSorted)
}

func testSorted(t *testing.T) {
	sorted := NewSorted(".")
	defer sorted.Close()

	testValues := randomTestValues(testSize)
	testValuesSorted := append([]SortValue(nil), testValues...)
	sort.Sort(sortSlice(testValuesSorted))

	for i, tv := range testValues {
		if i%100 == 0 {
			if l := sorted.Length(); l != i {
				t.Errorf("s.Length() == %v, expected %v", l, i)
			}
			if s := sorted.Size(); s != int64(i)*10 {
				t.Errorf("s.Size() == %v, expected %v", s, i*10)
			}
		}
		sorted.Add(tv)
	}

	i := 0
	sorted.Iter(func(v SortValue) bool {
		tv := v.(*testValue).string
		if exp := testValuesSorted[i].(*testValue).string; tv != exp {
			t.Errorf("Iterating at %v: %v != %v", i, tv, exp)
			return false
		}
		i++
		return true
	}, false, &testValue{})
	if i != len(testValuesSorted) {
		t.Errorf("Received just %v files, expected %v", i, len(testValuesSorted))
	}

	if s := sorted.Size(); s != int64(len(testValues))*10 {
		t.Errorf("s.Size() == %v, expected %v", s, len(testValues)*10)
	}
	if l := sorted.Length(); l != len(testValues) {
		t.Errorf("s.Length() == %v, expected %v", l, len(testValues))
	}

	v, ok := sorted.PopFirst(&testValue{})
	if !ok {
		t.Fatalf("PopFirst didn't return any value")
	}
	got := v.(*testValue).string
	if exp := testValuesSorted[0].(*testValue).string; got != exp {
		t.Errorf("PopFirst: %v != %v", got, exp)
	}
	if s := sorted.Size(); s != int64(len(testValues)-1)*10 {
		t.Errorf("s.Size() == %v, expected %v", s, (len(testValues)-1)*10)
	}
	if l := sorted.Length(); l != len(testValues)-1 {
		t.Errorf("s.Length() == %v, expected %v", l, len(testValues)-1)
	}

	v, ok = sorted.PopLast(&testValue{})
	if !ok {
		t.Fatalf("PopLast didn't return any value")
	}
	got = v.(*testValue).string
	if exp := testValuesSorted[len(testValuesSorted)-1].(*testValue).string; got != exp {
		t.Errorf("PopLast: %v != %v", got, exp)
	}
	if s := sorted.Size(); s != int64(len(testValues)-2)*10 {
		t.Errorf("s.Size() == %v, expected %v", s, (len(testValues)-2)*10)
	}
	if l := sorted.Length(); l != len(testValues)-2 {
		t.Errorf("s.Length() == %v, expected %v", l, len(testValues)-2)
	}

	i = len(testValues) - 1
	sorted.IterAndClose(func(v SortValue) bool {
		i--
		tv := v.(*testValue).string
		if exp := testValuesSorted[i].(*testValue).string; tv != exp {
			t.Errorf("Iterating at %v: %v != %v", i, tv, exp)
			return false
		}
		return true
	}, true, &testValue{})
	if i != 1 {
		t.Errorf("Last received file at index %v, should have gone to 1", i)
	}
}

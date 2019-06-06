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
	withAdjustedMem(t, 0, testSorted)
}

func TestSorted100B(t *testing.T) {
	withAdjustedMem(t, 100, testSorted)
}

func TestSorted100kB(t *testing.T) {
	withAdjustedMem(t, 100000, testSorted)
}

func testSorted(t *testing.T) {
	sorted := NewSortedMap(".")
	defer sorted.Close()

	testValues := randomTestValues(testSize)
	testValuesSorted := make([]string, 0, testSize)
	for _, tv := range testValues {
		testValuesSorted = append(testValuesSorted, tv.string)
	}
	sort.Strings(testValuesSorted)

	for i, tv := range testValues {
		if i%100 == 0 {
			if l := sorted.Items(); l != i {
				t.Fatalf("s.Items() == %v, expected %v", l, i)
			}
			if s := sorted.Bytes(); s != i*10 {
				t.Fatalf("s.Bytes() == %v, expected %v", s, i*10)
			}
		}
		sorted.Set([]byte(tv.string), tv)
	}

	i := 0
	it := sorted.NewIterator()
	v := &testValue{}
	for it.Next() {
		it.Value(v)
		tv := v.string
		if exp := testValuesSorted[i]; tv != exp {
			t.Fatalf("Iterating at %v: %v != %v", i, tv, exp)
			break
		}
		i++
	}
	it.Release()
	if i != len(testValuesSorted) {
		t.Fatalf("Received just %v files, expected %v", i, len(testValuesSorted))
	}

	if s := sorted.Bytes(); s != len(testValues)*10 {
		t.Fatalf("s.Bytes() == %v, expected %v", s, len(testValues)*10)
	}
	if l := sorted.Items(); l != len(testValues) {
		t.Fatalf("s.Items() == %v, expected %v", l, len(testValues))
	}

	v.Reset()
	ok := sorted.PopFirst(v)
	if !ok {
		t.Fatalf("PopFirst didn't return any value")
	}
	got := v.string
	if exp := testValuesSorted[0]; got != exp {
		t.Fatalf("PopFirst: %v != %v", got, exp)
	}
	if s := sorted.Bytes(); s != (len(testValues)-1)*10 {
		t.Fatalf("s.Bytes() == %v, expected %v", s, (len(testValues)-1)*10)
	}
	if l := sorted.Items(); l != len(testValues)-1 {
		t.Fatalf("s.Items() == %v, expected %v", l, len(testValues)-1)
	}

	v.Reset()
	ok = sorted.PopLast(v)
	if !ok {
		t.Fatalf("PopLast didn't return any value")
	}
	got = v.string
	if exp := testValuesSorted[len(testValuesSorted)-1]; got != exp {
		t.Fatalf("PopLast: %v != %v", got, exp)
	}
	if s := sorted.Bytes(); s != (len(testValues)-2)*10 {
		t.Fatalf("s.Bytes() == %v, expected %v", s, (len(testValues)-2)*10)
	}
	if l := sorted.Items(); l != len(testValues)-2 {
		t.Fatalf("s.Items() == %v, expected %v", l, len(testValues)-2)
	}

	i = len(testValues) - 1
	it = sorted.NewReverseIterator()
	for it.Next() {
		i--
		v.Reset()
		it.Value(v)
		tv := v.string
		if exp := testValuesSorted[i]; tv != exp {
			t.Fatalf("Iterating at %v: %v != %v", i, tv, exp)
			break
		}
	}
	it.Release()
	if i != 1 {
		t.Fatalf("Last received file at index %v, should have gone to 1", i)
	}
}

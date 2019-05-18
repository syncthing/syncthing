// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package diskoverflow

import (
	"math/rand"
	"testing"
)

const testSize = 10000

func withAdjustedMem(t *testing.T, mem int, fn func(t *testing.T)) {
	SetDefaultOverflowBytes(mem)

	fn(t)

	SetDefaultOverflowBytes(OrigDefaultOverflowBytes)
}

func TestSliceReal(t *testing.T) {
	testSlice(t)
}

func TestSliceNoMem(t *testing.T) {
	withAdjustedMem(t, 0, testSlice)
}

func TestSlice100B(t *testing.T) {
	withAdjustedMem(t, 100, testSlice)
}

func TestSlice100kB(t *testing.T) {
	withAdjustedMem(t, 100000, testSlice)
}

func testSlice(t *testing.T) {
	slice := NewSlice(".")
	defer slice.Close()

	testValues := randomTestValues(testSize)

	for i, tv := range testValues {
		if i%100 == 0 {
			if l := slice.Items(); l != i {
				t.Fatalf("s.Items() == %v, expected %v", l, i)
			}
			if s := slice.Bytes(); s != i*10 {
				t.Fatalf("s.Bytes() == %v, expected %v", s, i*10)
			}
		}
		slice.Append(tv)
	}

	i := 0
	it := slice.NewIterator()
	v := &testValue{}
	for it.Next() {
		it.Value(v)
		tv := v.string
		if exp := testValues[i].string; tv != exp {

			t.Fatalf("Iterating at %v: got %v, expected %v", i, tv, exp)
			break
		}
		i++
	}
	it.Release()
	if i != len(testValues) {
		t.Fatalf("Received just %v files, expected %v", i, len(testValues))
	}

	if s := slice.Bytes(); s != len(testValues)*10 {
		t.Fatalf("s.Bytes() == %v, expected %v", s, len(testValues)*10)
	}

	it = slice.NewReverseIterator()

	for it.Next() {
		i--
		v.Reset()
		it.Value(v)
		tv := v.string
		exp := testValues[i].string
		if tv != exp {
			t.Fatalf("Iterating at %v: got %v, expected %v", i, tv, exp)
			break
		}
	}
	it.Release()

	i = len(testValues)
	it = slice.NewReverseIterator()
	for it.Next() {
		i--
		v.Reset()
		it.Value(v)
		tv := v.string
		exp := testValues[i].string
		if tv != exp {
			t.Fatalf("Iterating at %v: got %v, expected %v", i, tv, exp)
			break
		}
	}
	it.Release()
	if i != 0 {
		t.Fatalf("Last received file at index %v, should have gone to 0", i)
	}
}

type testValue struct {
	string
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// https://stackoverflow.com/a/31832326/3864852
func randomTestValues(length int) []*testValue {
	l := make([]*testValue, length)
	for k := 0; k < length; k++ {
		b := make([]byte, 10)
		for i := range b {
			b[i] = letterBytes[rand.Int63()%int64(len(letterBytes))]
		}
		l[k] = &testValue{string(b)}
	}
	return l
}

func (t *testValue) ProtoSize() int {
	return len(t.string)
}

func (t *testValue) Marshal() ([]byte, error) {
	return []byte(t.string), nil
}

func (t *testValue) Unmarshal(v []byte) error {
	t.string = string(v)
	return nil
}

func (t *testValue) Reset() {
	t.string = ""
}

func (t *testValue) Key() []byte {
	return []byte(t.string)
}

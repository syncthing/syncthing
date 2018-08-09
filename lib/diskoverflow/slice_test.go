// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package diskoverflow

import (
	"math/rand"
	"os"
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
)

const testSize = 10000

func TestMain(m *testing.M) {
	oldMinCompactionSize := minCompactionSize
	minCompactionSize = 10 << protocol.KiB

	exitCode := m.Run()

	minCompactionSize = oldMinCompactionSize

	os.Exit(exitCode)
}

func withAdjustedMem(t *testing.T, mem int64, fn func(t *testing.T)) {
	oldMem := availableMemory
	availableMemory = mem
	setLimits()

	fn(t)

	availableMemory = oldMem
	setLimits()
}

func TestSliceReal(t *testing.T) {
	testSlice(t)
}

func TestSliceNoMem(t *testing.T) {
	withAdjustedMem(t, int64(0), testSlice)
}

func TestSlice100B(t *testing.T) {
	withAdjustedMem(t, int64(100), testSlice)
}

func TestSlice100kB(t *testing.T) {
	withAdjustedMem(t, int64(100000), testSlice)
}

func testSlice(t *testing.T) {
	slice := NewSlice(".", &testValue{})
	defer slice.Close()

	testValues := randomTestValues(testSize)

	for i, tv := range testValues {
		if i%100 == 0 {
			if l := slice.Length(); l != i {
				t.Errorf("s.Length() == %v, expected %v", l, i)
			}
			if s := slice.Size(); s != int64(i)*10 {
				t.Errorf("s.Size() == %v, expected %v", s, i*10)
			}
		}
		if i == 0 {
		}
		slice.Append(tv)
	}

	i := 0
	it := slice.NewIterator(false)
	for it.Next() {
		tv := it.Value().(*testValue).string
		if exp := testValues[i].(*testValue).string; tv != exp {

			t.Errorf("Iterating at %v: got %v, expected %v", i, tv, exp)
			break
		}
		i++
	}
	it.Release()
	if i != len(testValues) {
		t.Errorf("Received just %v files, expected %v", i, len(testValues))
	}

	if s := slice.Size(); s != int64(len(testValues))*10 {
		t.Errorf("s.Size() == %v, expected %v", s, len(testValues)*10)
	}

	it = slice.NewIterator(true)

	for it.Next() {
		i--
		tv := it.Value().(*testValue).string
		exp := testValues[i].(*testValue).string
		if tv != exp {
			t.Errorf("Iterating at %v: got %v, expected %v", i, tv, exp)
			break
		}
	}
	it.Release()

	i = len(testValues)
	it = slice.NewIterator(true)
	for it.Next() {
		i--
		tv := it.Value().(*testValue).string
		exp := testValues[i].(*testValue).string
		if tv != exp {
			t.Errorf("Iterating at %v: got %v, expected %v", i, tv, exp)
			break
		}
	}
	it.Release()
	slice.Close()
	if i != 0 {
		t.Errorf("Last received file at index %v, should have gone to 0", i)
	}
}

type testValue struct {
	string
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// https://stackoverflow.com/a/31832326/3864852
func randomTestValues(length int) []SortValue {
	l := make([]SortValue, length)
	for k := 0; k < length; k++ {
		b := make([]byte, 10)
		for i := range b {
			b[i] = letterBytes[rand.Int63()%int64(len(letterBytes))]
		}
		l[k] = &testValue{string(b)}
	}
	return l
}

func (t *testValue) Size() int64 {
	return int64(len(t.string))
}

func (t *testValue) Marshal() []byte {
	return []byte(t.string)
}

func (t *testValue) Unmarshal(v []byte) Value {
	return &testValue{string(v)}
}

func (t *testValue) UnmarshalWithKey(_, v []byte) SortValue {
	return t.Unmarshal(v).(SortValue)
}

func (t *testValue) Key() []byte {
	return []byte(t.string)
}

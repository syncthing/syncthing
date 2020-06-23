// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package contract

import (
	"reflect"
	"testing"
)

type PtrStruct struct {
	A string         `since:"2"`
	B map[string]int `since:"3"`
}

type Nested struct {
	A float32 `since:"4"`
	B [4]int  `since:"5"`
	C bool    `since:"1"`
}

type TestStruct struct {
	A      int
	B      map[string]string `since:"1"`
	C      []string          `since:"2"`
	Nested Nested            `since:"3"`

	Ptr *PtrStruct `since:"2"`
}

func testValue() TestStruct {
	return TestStruct{
		A: 1,
		B: map[string]string{
			"foo": "bar",
		},
		C: []string{"a", "b"},
		Nested: Nested{
			A: 0.10,
			B: [4]int{1, 2, 3, 4},
			C: true,
		},
		Ptr: &PtrStruct{
			A: "value",
			B: map[string]int{
				"x": 1,
				"b": 2,
			},
		},
	}
}

func TestClean(t *testing.T) {
	expect(t, 0, TestStruct{})

	expect(t, 1, TestStruct{
		// A unset, since it does not have "since"
		B: map[string]string{
			"foo": "bar",
		},
	})

	expect(t, 2, TestStruct{
		// A unset, since it does not have "since"
		B: map[string]string{
			"foo": "bar",
		},
		C: []string{"a", "b"},
		Ptr: &PtrStruct{
			A: "value",
		},
	})

	expect(t, 3, TestStruct{
		// A unset, since it does not have "since"
		B: map[string]string{
			"foo": "bar",
		},
		C: []string{"a", "b"},
		Nested: Nested{
			C: true,
		},
		Ptr: &PtrStruct{
			A: "value",
			B: map[string]int{
				"x": 1,
				"b": 2,
			},
		},
	})

	expect(t, 4, TestStruct{
		// A unset, since it does not have "since"
		B: map[string]string{
			"foo": "bar",
		},
		C: []string{"a", "b"},
		Nested: Nested{
			A: 0.10,
			C: true,
		},
		Ptr: &PtrStruct{
			A: "value",
			B: map[string]int{
				"x": 1,
				"b": 2,
			},
		},
	})

	x := testValue()
	x.A = 0

	expect(t, 5, x)
	expect(t, 6, x)
}

func expect(t *testing.T, since int, b interface{}) {
	t.Helper()
	x := testValue()
	if err := clear(&x, since); err != nil {
		t.Fatal(err.Error())
	}
	if !reflect.DeepEqual(x, b) {
		t.Errorf("%#v != %#v", x, b)
	}
}

func TestMarshallingBehaviour(t *testing.T) {
	r := Report{}

	if err := r.Scan([]byte(`{"folderUses":{"sendonly": 100}}`)); err != nil {
		t.Fatal(err)
	}

	if r.FolderUses.SendOnly != 100 {
		t.Errorf("%d != 100", r.FolderUses.SendOnly)
	}

	if err := r.Scan([]byte(`{"folderUses":{"sendreceive": 200}}`)); err != nil {
		t.Fatal(err)
	}

	if r.FolderUses.SendReceive != 200 {
		t.Errorf("%d != 200", r.FolderUses.SendReceive)
	}

	if r.FolderUses.SendOnly != 0 {
		t.Errorf("%d != 0", r.FolderUses.SendOnly)
	}
}

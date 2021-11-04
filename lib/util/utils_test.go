// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package util

import (
	"testing"
)

type Defaulter struct {
	Value string
}

func (d *Defaulter) ParseDefault(v string) error {
	*d = Defaulter{Value: v}
	return nil
}

func TestSetDefaults(t *testing.T) {
	x := &struct {
		A string    `default:"string"`
		B int       `default:"2"`
		C float64   `default:"2.2"`
		D bool      `default:"true"`
		E Defaulter `default:"defaulter"`
	}{}

	if x.A != "" {
		t.Error("string failed")
	} else if x.B != 0 {
		t.Error("int failed")
	} else if x.C != 0 {
		t.Errorf("float failed")
	} else if x.D {
		t.Errorf("bool failed")
	} else if x.E.Value != "" {
		t.Errorf("defaulter failed")
	}

	SetDefaults(x)

	if x.A != "string" {
		t.Error("string failed")
	} else if x.B != 2 {
		t.Error("int failed")
	} else if x.C != 2.2 {
		t.Errorf("float failed")
	} else if !x.D {
		t.Errorf("bool failed")
	} else if x.E.Value != "defaulter" {
		t.Errorf("defaulter failed")
	}
}

func TestUniqueStrings(t *testing.T) {
	tests := []struct {
		input    []string
		expected []string
	}{
		{
			[]string{"a", "b"},
			[]string{"a", "b"},
		},
		{
			[]string{"a", "a"},
			[]string{"a"},
		},
		{
			[]string{"a", "a", "a", "a"},
			[]string{"a"},
		},
		{
			nil,
			nil,
		},
		{
			[]string{"       a     ", "     a  ", "b        ", "    b"},
			[]string{"a", "b"},
		},
	}

	for _, test := range tests {
		result := UniqueTrimmedStrings(test.input)
		if len(result) != len(test.expected) {
			t.Errorf("%s != %s", result, test.expected)
		}
		for i := range result {
			if test.expected[i] != result[i] {
				t.Errorf("%s != %s", result, test.expected)
			}
		}
	}
}

func TestFillNillSlices(t *testing.T) {
	// Nil
	x := &struct {
		A []string `default:"a,b"`
	}{}

	if x.A != nil {
		t.Error("not nil")
	}

	if err := FillNilSlices(x); err != nil {
		t.Error(err)
	}

	if len(x.A) != 2 {
		t.Error("length")
	}

	// Already provided
	y := &struct {
		A []string `default:"c,d,e"`
	}{[]string{"a", "b"}}

	if len(y.A) != 2 {
		t.Error("length")
	}

	if err := FillNilSlices(y); err != nil {
		t.Error(err)
	}

	if len(y.A) != 2 {
		t.Error("length")
	}

	// Non-nil but empty
	z := &struct {
		A []string `default:"c,d,e"`
	}{[]string{}}

	if len(z.A) != 0 {
		t.Error("length")
	}

	if err := FillNilSlices(z); err != nil {
		t.Error(err)
	}

	if len(z.A) != 0 {
		t.Error("length")
	}
}

func TestAddress(t *testing.T) {
	tests := []struct {
		network string
		host    string
		result  string
	}{
		{"tcp", "google.com", "tcp://google.com"},
		{"foo", "google", "foo://google"},
		{"123", "456", "123://456"},
	}

	for _, test := range tests {
		result := Address(test.network, test.host)
		if result != test.result {
			t.Errorf("%s != %s", result, test.result)
		}
	}
}

func TestCopyMatching(t *testing.T) {
	type Nested struct {
		A int
	}
	type Test struct {
		CopyA  int
		CopyB  []string
		CopyC  Nested
		CopyD  *Nested
		NoCopy int `restart:"true"`
	}

	from := Test{
		CopyA: 1,
		CopyB: []string{"friend", "foe"},
		CopyC: Nested{
			A: 2,
		},
		CopyD: &Nested{
			A: 3,
		},
		NoCopy: 4,
	}

	to := Test{
		CopyA: 11,
		CopyB: []string{"foot", "toe"},
		CopyC: Nested{
			A: 22,
		},
		CopyD: &Nested{
			A: 33,
		},
		NoCopy: 44,
	}

	// Copy empty fields
	CopyMatchingTag(&from, &to, "restart", func(v string) bool {
		return v != "true"
	})

	if to.CopyA != 1 {
		t.Error("CopyA")
	}
	if len(to.CopyB) != 2 || to.CopyB[0] != "friend" || to.CopyB[1] != "foe" {
		t.Error("CopyB")
	}
	if to.CopyC.A != 2 {
		t.Error("CopyC")
	}
	if to.CopyD.A != 3 {
		t.Error("CopyC")
	}
	if to.NoCopy != 44 {
		t.Error("NoCopy")
	}
}

func TestFillNil(t *testing.T) {
	type A struct {
		Slice []int
		Map   map[string]string
		Chan  chan int
	}

	type B struct {
		Slice *[]int
		Map   *map[string]string
		Chan  *chan int
	}

	type C struct {
		A A
		B *B
		D *****[]int
	}

	c := C{}
	FillNil(&c)

	if c.A.Slice == nil {
		t.Error("c.A.Slice")
	}
	if c.A.Map == nil {
		t.Error("c.A.Slice")
	}
	if c.A.Chan == nil {
		t.Error("c.A.Chan")
	}
	if c.B == nil {
		t.Error("c.B")
	}
	if c.B.Slice == nil {
		t.Error("c.B.Slice")
	}
	if c.B.Map == nil {
		t.Error("c.B.Slice")
	}
	if c.B.Chan == nil {
		t.Error("c.B.Chan")
	}
	if *c.B.Slice == nil {
		t.Error("*c.B.Slice")
	}
	if *c.B.Map == nil {
		t.Error("*c.B.Slice")
	}
	if *c.B.Chan == nil {
		t.Error("*c.B.Chan")
	}
	if *****c.D == nil {
		t.Error("c.D")
	}
}

func TestFillNilDoesNotBulldozeSetFields(t *testing.T) {
	type A struct {
		Slice []int
		Map   map[string]string
		Chan  chan int
	}

	type B struct {
		Slice *[]int
		Map   *map[string]string
		Chan  *chan int
	}

	type C struct {
		A A
		B *B
		D **[]int
	}

	ch := make(chan int, 10)
	d := make([]int, 10)
	dd := &d

	c := C{
		A: A{
			Slice: []int{1},
			Map: map[string]string{
				"k": "v",
			},
			Chan: make(chan int, 10),
		},
		B: &B{
			Slice: &[]int{1},
			Map: &map[string]string{
				"k": "v",
			},
			Chan: &ch,
		},
		D: &dd,
	}
	FillNil(&c)

	if len(c.A.Slice) != 1 {
		t.Error("c.A.Slice")
	}
	if len(c.A.Map) != 1 {
		t.Error("c.A.Slice")
	}
	if cap(c.A.Chan) != 10 {
		t.Error("c.A.Chan")
	}
	if c.B == nil {
		t.Error("c.B")
	}
	if len(*c.B.Slice) != 1 {
		t.Error("c.B.Slice")
	}
	if len(*c.B.Map) != 1 {
		t.Error("c.B.Slice")
	}
	if cap(*c.B.Chan) != 10 {
		t.Error("c.B.Chan")
	}
	if cap(**c.D) != 10 {
		t.Error("c.D")
	}
}

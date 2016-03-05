// Copyright (C) 2014 Jakob Borg. All rights reserved. Use of this source code
// is governed by an MIT-style license that can be found in the LICENSE file.

package xdr_test

import "testing"

type XDRBenchStruct struct {
	I1  uint64
	I2  uint32
	I3  uint16
	I4  uint8
	Bs0 []byte // max:128
	Bs1 []byte
	Is0 []int32
	S0  string // max:128
	S1  string
}

var res []byte // not to be optimized away
var s = XDRBenchStruct{
	I1:  42,
	I2:  43,
	I3:  44,
	I4:  45,
	Bs0: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18},
	Bs1: []byte{11, 12, 13, 14, 15, 16, 17, 18, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	Is0: []int32{23, 43},
	S0:  "Hello World! String one.",
	S1:  "Hello World! String two.",
}

func BenchmarkThisMarshal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		res, _ = s.MarshalXDR()
	}

	b.ReportAllocs()
}

func BenchmarkThisUnmarshal(b *testing.B) {
	bs := s.MustMarshalXDR()
	var t XDRBenchStruct
	for i := 0; i < b.N; i++ {
		err := t.UnmarshalXDR(bs)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
}

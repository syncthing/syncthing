// Copyright (C) 2014 Jakob Borg and other contributors. All rights reserved.
// Use of this source code is governed by an MIT-style license that can be
// found in the LICENSE file.

package xdr_test

import (
	"bytes"
	"testing"
)

type XDRBenchStruct struct {
	I1 uint64
	I2 uint32
	I3 uint16
	Bs []byte
	S  string
}

var res []byte // no to be optimized away
var s = XDRBenchStruct{
	I1: 42,
	I2: 43,
	I3: 44,
	Bs: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18},
	S:  "Hello World!",
}
var e = s.MarshalXDR()

func BenchmarkThisMarshal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		res = s.MarshalXDR()
	}
}

func BenchmarkThisUnmarshal(b *testing.B) {
	var t XDRBenchStruct
	for i := 0; i < b.N; i++ {
		err := t.UnmarshalXDR(e)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncode(b *testing.B) {
	bs := make([]byte, 0, 65536)
	buf := bytes.NewBuffer(bs)

	for i := 0; i < b.N; i++ {
		_, err := s.EncodeXDR(buf)
		if err != nil {
			b.Fatal(err)
		}
		buf.Reset()
	}
}

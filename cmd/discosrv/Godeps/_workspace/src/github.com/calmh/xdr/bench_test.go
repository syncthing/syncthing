// Copyright (C) 2014 Jakob Borg. All rights reserved. Use of this source code
// is governed by an MIT-style license that can be found in the LICENSE file.

package xdr_test

import (
	"io"
	"io/ioutil"
	"testing"

	"github.com/calmh/xdr"
)

type XDRBenchStruct struct {
	I1  uint64
	I2  uint32
	I3  uint16
	I4  uint8
	Bs0 []byte // max:128
	Bs1 []byte
	S0  string // max:128
	S1  string
}

var res []byte // no to be optimized away
var s = XDRBenchStruct{
	I1:  42,
	I2:  43,
	I3:  44,
	I4:  45,
	Bs0: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18},
	Bs1: []byte{11, 12, 13, 14, 15, 16, 17, 18, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	S0:  "Hello World! String one.",
	S1:  "Hello World! String two.",
}
var e []byte

func init() {
	e, _ = s.MarshalXDR()
}

func BenchmarkThisMarshal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		res, _ = s.MarshalXDR()
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

func BenchmarkThisEncode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := s.EncodeXDR(ioutil.Discard)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkThisEncoder(b *testing.B) {
	w := xdr.NewWriter(ioutil.Discard)
	for i := 0; i < b.N; i++ {
		_, err := s.EncodeXDRInto(w)
		if err != nil {
			b.Fatal(err)
		}
	}
}

type repeatReader struct {
	data []byte
}

func (r *repeatReader) Read(bs []byte) (n int, err error) {
	if len(bs) > len(r.data) {
		err = io.EOF
	}
	n = copy(bs, r.data)
	r.data = r.data[n:]
	return n, err
}

func (r *repeatReader) Reset(bs []byte) {
	r.data = bs
}

func BenchmarkThisDecode(b *testing.B) {
	rr := &repeatReader{e}
	var t XDRBenchStruct
	for i := 0; i < b.N; i++ {
		err := t.DecodeXDR(rr)
		if err != nil {
			b.Fatal(err)
		}
		rr.Reset(e)
	}
}

func BenchmarkThisDecoder(b *testing.B) {
	rr := &repeatReader{e}
	r := xdr.NewReader(rr)
	var t XDRBenchStruct
	for i := 0; i < b.N; i++ {
		err := t.DecodeXDRFrom(r)
		if err != nil {
			b.Fatal(err)
		}
		rr.Reset(e)
	}
}

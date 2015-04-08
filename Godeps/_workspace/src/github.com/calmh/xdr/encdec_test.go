// Copyright (C) 2014 Jakob Borg. All rights reserved. Use of this source code
// is governed by an MIT-style license that can be found in the LICENSE file.

package xdr_test

import (
	"bytes"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/calmh/xdr"
)

// Contains all supported types
type TestStruct struct {
	I    int
	I8   int8
	UI8  uint8
	I16  int16
	UI16 uint16
	I32  int32
	UI32 uint32
	I64  int64
	UI64 uint64
	BS   []byte // max:1024
	S    string // max:1024
	C    Opaque
	SS   []string // max:1024
}

type Opaque [32]byte

func (u *Opaque) EncodeXDRInto(w *xdr.Writer) (int, error) {
	return w.WriteRaw(u[:])
}

func (u *Opaque) DecodeXDRFrom(r *xdr.Reader) (int, error) {
	return r.ReadRaw(u[:])
}

func (Opaque) Generate(rand *rand.Rand, size int) reflect.Value {
	var u Opaque
	for i := range u[:] {
		u[i] = byte(rand.Int())
	}
	return reflect.ValueOf(u)
}

func TestEncDec(t *testing.T) {
	fn := func(t0 TestStruct) bool {
		bs, err := t0.MarshalXDR()
		if err != nil {
			t.Fatal(err)
		}
		var t1 TestStruct
		err = t1.UnmarshalXDR(bs)
		if err != nil {
			t.Fatal(err)
		}

		// Not comparing with DeepEqual since we'll unmarshal nil slices as empty
		if t0.I != t1.I ||
			t0.I16 != t1.I16 || t0.UI16 != t1.UI16 ||
			t0.I32 != t1.I32 || t0.UI32 != t1.UI32 ||
			t0.I64 != t1.I64 || t0.UI64 != t1.UI64 ||
			bytes.Compare(t0.BS, t1.BS) != 0 ||
			t0.S != t1.S || t0.C != t1.C {
			t.Logf("%#v", t0)
			t.Logf("%#v", t1)
			return false
		}
		return true
	}
	if err := quick.Check(fn, nil); err != nil {
		t.Error(err)
	}
}

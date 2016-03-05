// Copyright (C) 2014 Jakob Borg. All rights reserved. Use of this source code
// is governed by an MIT-style license that can be found in the LICENSE file.

package xdr_test

import (
	"bytes"
	"io"
	"log"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/calmh/xdr"
)

// Contains all supported types
type TestStruct struct {
	B    bool
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
	ES   EmptyStruct
	OS   OtherStruct
	OSs  []OtherStruct
}

func (s1 TestStruct) TestEquals(s2 TestStruct) bool {
	if s1.B != s2.B {
		log.Printf("B differ; %v != %v", s1.B, s2.B)
		return false
	}
	if s1.I != s2.I {
		log.Printf("I differ; %d != %d", s1.I, s2.I)
		return false
	}
	if s1.I8 != s2.I8 {
		log.Printf("I8 differ; %d != %d", s1.I8, s2.I8)
		return false
	}
	if s1.UI8 != s2.UI8 {
		log.Printf("UI8 differ; %d != %d", s1.UI8, s2.UI8)
		return false
	}
	if s1.I16 != s2.I16 {
		log.Printf("I16 differ; %d != %d", s1.I16, s2.I16)
		return false
	}
	if s1.UI16 != s2.UI16 {
		log.Printf("UI16 differ; %d != %d", s1.UI16, s2.UI16)
		return false
	}
	if s1.I32 != s2.I32 {
		log.Printf("I32 differ; %d != %d", s1.I32, s2.I32)
		return false
	}
	if s1.UI32 != s2.UI32 {
		log.Printf("UI32 differ; %d != %d", s1.UI32, s2.UI32)
		return false
	}
	if s1.I64 != s2.I64 {
		log.Printf("I64 differ; %d != %d", s1.I64, s2.I64)
		return false
	}
	if s1.UI64 != s2.UI64 {
		log.Printf("UI64 differ; %d != %d", s1.UI64, s2.UI64)
		return false
	}
	if !bytes.Equal(s1.BS, s2.BS) {
		log.Println("BS differ")
		return false
	}
	if s1.S != s2.S {
		log.Printf("S differ; %q != %q", s1.S, s2.S)
		return false
	}
	if s1.C != s2.C {
		log.Printf("C differ; %q != %q", s1.C, s2.C)
		return false
	}
	if len(s1.SS) != len(s2.SS) {
		log.Printf("len(SS) differ; %q != %q", len(s1.SS), len(s2.SS))
		return false
	}
	for i := range s1.SS {
		if s1.SS[i] != s2.SS[i] {
			log.Printf("SS[%d] differ; %q != %q", i, s1.SS[i], s2.SS[i])
			return false
		}
	}
	if s1.OS != s2.OS {
		log.Printf("OS differ; %q != %q", s1.OS, s2.OS)
		return false
	}
	if len(s1.OSs) != len(s2.OSs) {
		log.Printf("len(OSs) differ; %q != %q", len(s1.OSs), len(s2.OSs))
		return false
	}
	for i := range s1.OSs {
		if s1.OSs[i] != s2.OSs[i] {
			log.Printf("OSs[%d] differ; %q != %q", i, s1.OSs[i], s2.OSs[i])
			return false
		}
	}

	return true
}

type EmptyStruct struct {
}

type OtherStruct struct {
	F1 uint32
	F2 string
}

type Opaque [32]byte

func (u *Opaque) XDRSize() int {
	return 32
}

func (u *Opaque) MarshalXDRInto(m *xdr.Marshaller) error {
	m.MarshalRaw(u[:])
	return m.Error
}

func (o *Opaque) UnmarshalXDRFrom(u *xdr.Unmarshaller) error {
	copy((*o)[:], u.UnmarshalRaw(32))
	return u.Error
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

		return t0.TestEquals(t1)
	}
	if err := quick.Check(fn, nil); err != nil {
		t.Error(err)
	}
}

func TestMarshalShortBuffer(t *testing.T) {
	var s TestStruct
	buf := make([]byte, s.XDRSize())
	if err := s.MarshalXDRInto(&xdr.Marshaller{Data: buf}); err != nil {
		t.Fatal("Unexpected error", err)
	}

	if err := s.MarshalXDRInto(&xdr.Marshaller{Data: buf[1:]}); err != io.ErrShortBuffer {
		t.Fatal("Expected io.ErrShortBuffer, got", err)
	}
}

func TestUnmarshalUnexpectedEOF(t *testing.T) {
	var s TestStruct
	buf := make([]byte, s.XDRSize())
	if err := s.MarshalXDRInto(&xdr.Marshaller{Data: buf}); err != nil {
		t.Fatal("Unexpected error", err)
	}

	if err := s.UnmarshalXDR(buf[:len(buf)-1]); err != io.ErrUnexpectedEOF {
		t.Fatal("Expected io.ErrUnexpectedEOF, got", err)
	}

	u := &xdr.Unmarshaller{Data: buf[:3]}
	u.UnmarshalRaw(4)
	if err := u.Error; err != io.ErrUnexpectedEOF {
		t.Fatal("Expected io.ErrUnexpectedEOF, got", err)
	}

	u = &xdr.Unmarshaller{Data: buf[:3]}
	u.UnmarshalString()
	if err := u.Error; err != io.ErrUnexpectedEOF {
		t.Fatal("Expected io.ErrUnexpectedEOF, got", err)
	}

	u = &xdr.Unmarshaller{Data: buf[:3]}
	u.UnmarshalBytes()
	if err := u.Error; err != io.ErrUnexpectedEOF {
		t.Fatal("Expected io.ErrUnexpectedEOF, got", err)
	}

	u = &xdr.Unmarshaller{Data: buf[:3]}
	u.UnmarshalBool()
	if err := u.Error; err != io.ErrUnexpectedEOF {
		t.Fatal("Expected io.ErrUnexpectedEOF, got", err)
	}

	u = &xdr.Unmarshaller{Data: buf[:3]}
	u.UnmarshalUint8()
	if err := u.Error; err != io.ErrUnexpectedEOF {
		t.Fatal("Expected io.ErrUnexpectedEOF, got", err)
	}

	u = &xdr.Unmarshaller{Data: buf[:3]}
	u.UnmarshalUint16()
	if err := u.Error; err != io.ErrUnexpectedEOF {
		t.Fatal("Expected io.ErrUnexpectedEOF, got", err)
	}

	u = &xdr.Unmarshaller{Data: buf[:3]}
	u.UnmarshalUint32()
	if err := u.Error; err != io.ErrUnexpectedEOF {
		t.Fatal("Expected io.ErrUnexpectedEOF, got", err)
	}

	u = &xdr.Unmarshaller{Data: buf[:7]}
	u.UnmarshalUint64()
	if err := u.Error; err != io.ErrUnexpectedEOF {
		t.Fatal("Expected io.ErrUnexpectedEOF, got", err)
	}
}
